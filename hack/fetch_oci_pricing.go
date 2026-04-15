package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type rootObject struct {
	Items []map[string]any `json:"items"`
}

func main() {
	var (
		endpoint      string
		currency      string
		mappingPath   string
		outPath       string
		modelContains string
	)

	flag.StringVar(&endpoint, "endpoint", "https://apexapps.oracle.com/pls/apex/cetools/api/v1/products/", "OCI list-pricing endpoint")
	flag.StringVar(&currency, "currency", "USD", "Currency code for pricing query")
	flag.StringVar(&mappingPath, "mapping", "pkg/pricing/oci_part_numbers.json", "JSON file mapping shape (or exact instance-type key) -> OCI part number")
	flag.StringVar(&outPath, "out", "pkg/pricing/static_prices.json", "Output JSON file for static prices")
	flag.StringVar(&modelContains, "model-contains", "hour", "Filter pricing model text by substring, empty to disable")
	flag.Parse()

	// Note:
	// OCI Flex pricing is usually published as separate OCPU and memory items.
	// This helper fetches one numeric value per mapping key, so for high-fidelity
	// Flex pricing prefer generating a per-instance-type pricing file where each
	// value is precomputed as:
	//   hourly = (ocpus * ocpu_rate) + (memory_gb * memory_rate)
	// and pass it to the viewer via --pricing-file.

	mappingBytes, err := os.ReadFile(mappingPath)
	if err != nil {
		fatalf("read mapping file: %v", err)
	}

	shapeToPart := map[string]string{}
	if err := json.Unmarshal(mappingBytes, &shapeToPart); err != nil {
		fatalf("parse mapping file: %v", err)
	}
	if len(shapeToPart) == 0 {
		fatalf("mapping file %q is empty", mappingPath)
	}

	client := &http.Client{Timeout: 20 * time.Second}
	shapeToPrice := map[string]float64{}

	var shapes []string
	for shape := range shapeToPart {
		shapes = append(shapes, shape)
	}
	sort.Strings(shapes)

	for _, shape := range shapes {
		partNumber := strings.TrimSpace(shapeToPart[shape])
		if partNumber == "" {
			fatalf("shape %q has empty part number", shape)
		}

		price, err := fetchPrice(client, endpoint, currency, partNumber, modelContains)
		if err != nil {
			fatalf("fetch price for %q (%s): %v", shape, partNumber, err)
		}
		shapeToPrice[shape] = price
	}

	out, err := json.MarshalIndent(shapeToPrice, "", "  ")
	if err != nil {
		fatalf("marshal output: %v", err)
	}
	out = append(out, '\n')

	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		fatalf("write output: %v", err)
	}

	fmt.Printf("wrote %d shape prices to %s\n", len(shapeToPrice), outPath)
}

func fetchPrice(client *http.Client, endpoint, currency, partNumber, modelContains string) (float64, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return 0, fmt.Errorf("parse endpoint: %w", err)
	}

	q := u.Query()
	q.Set("currencyCode", currency)
	q.Set("partNumber", partNumber)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "oke-node-viewer-pricing-fetcher/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}

	var parsed rootObject
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, fmt.Errorf("decode JSON: %w", err)
	}
	if len(parsed.Items) == 0 {
		return 0, fmt.Errorf("no pricing items returned")
	}

	modelContains = strings.ToLower(strings.TrimSpace(modelContains))
	for _, item := range parsed.Items {
		// Current OCI API layout: currencyCodeLocalizations[].prices[].{model,value}
		if value, ok := getNestedLocalizedPrice(item, currency, modelContains); ok {
			return value, nil
		}

		// Legacy/fallback format checks.
		model := strings.ToLower(getString(item, "model"))
		if modelContains != "" && !strings.Contains(model, modelContains) {
			continue
		}
		if value, ok := getFloat(item, "value"); ok {
			return value, nil
		}
	}

	for _, item := range parsed.Items {
		if value, ok := getFloat(item, "value"); ok {
			return value, nil
		}
	}
	return 0, fmt.Errorf("no numeric value found in pricing items")
}

func getNestedLocalizedPrice(item map[string]any, currency, modelContains string) (float64, bool) {
	rawLoc, ok := item["currencyCodeLocalizations"]
	if !ok || rawLoc == nil {
		return 0, false
	}
	locs, ok := rawLoc.([]any)
	if !ok {
		return 0, false
	}

	for _, locRaw := range locs {
		loc, ok := locRaw.(map[string]any)
		if !ok {
			continue
		}
		if c := strings.ToUpper(getString(loc, "currencyCode")); c != "" && c != strings.ToUpper(currency) {
			continue
		}
		rawPrices, ok := loc["prices"]
		if !ok || rawPrices == nil {
			continue
		}
		prices, ok := rawPrices.([]any)
		if !ok {
			continue
		}

		// Prefer entries that match modelContains.
		for _, prRaw := range prices {
			pr, ok := prRaw.(map[string]any)
			if !ok {
				continue
			}
			model := strings.ToLower(getString(pr, "model"))
			if modelContains != "" && !strings.Contains(model, modelContains) {
				continue
			}
			if v, ok := getFloat(pr, "value"); ok {
				return v, true
			}
		}

		// Fallback: first numeric price in this localization.
		for _, prRaw := range prices {
			pr, ok := prRaw.(map[string]any)
			if !ok {
				continue
			}
			if v, ok := getFloat(pr, "value"); ok {
				return v, true
			}
		}
	}

	return 0, false
}

func getString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprintf("%v", t)
	}
}

func getFloat(m map[string]any, key string) (float64, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
