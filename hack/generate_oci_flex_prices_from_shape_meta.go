package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type shapeMeta struct {
	Prices []shapePrice `json:"prices"`
}

type shapePrice struct {
	ShapeName       string  `json:"shapeName"`
	OcpuUnitPrice   float64 `json:"ocpuUnitPrice"`
	MemoryUnitPrice float64 `json:"memoryUnitPrice"`
}

type combo struct {
	ocpu int
	mem  int
}

func main() {
	var (
		shapeMetaPath string
		shapesCSV     string
		combosCSV     string
		outPath       string
		includeBase   bool
		baseCombo     string
	)

	defaultShapeMetaPath := "../karpenter-provider-oci/chart/config/oci-shape-meta.json"
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		defaultShapeMetaPath = filepath.Join(home, "karpenter-provider-oci", "chart", "config", "oci-shape-meta.json")
	}

	flag.StringVar(&shapeMetaPath, "shape-meta", defaultShapeMetaPath, "Path to KPO oci-shape-meta.json")
	flag.StringVar(&shapesCSV, "shapes", "VM.Standard.E3.Flex,VM.Standard.E4.Flex,VM.Standard.E5.Flex", "Comma-separated Flex shape names to include")
	flag.StringVar(&combosCSV, "combos", "1:4,2:8,4:16,8:32", "Comma-separated OCPU:MemoryGB combinations")
	flag.StringVar(&outPath, "out", "pkg/pricing/static_prices.json", "Output JSON file for static prices")
	flag.BoolVar(&includeBase, "include-base-shape-keys", true, "Include base shape keys (e.g. VM.Standard.E3.Flex) using the selected base combo")
	flag.StringVar(&baseCombo, "base-combo", "4:16", "Base combo used when include-base-shape-keys=true")
	flag.Parse()

	shapeSet := map[string]struct{}{}
	for _, s := range strings.Split(shapesCSV, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		shapeSet[s] = struct{}{}
	}
	if len(shapeSet) == 0 {
		fatalf("no shapes provided")
	}

	combos, err := parseCombos(combosCSV)
	if err != nil {
		fatalf("parse combos: %v", err)
	}
	base, err := parseCombo(baseCombo)
	if err != nil {
		fatalf("parse base combo: %v", err)
	}

	raw, err := os.ReadFile(shapeMetaPath)
	if err != nil {
		fatalf("read shape meta: %v", err)
	}
	var meta shapeMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		fatalf("parse shape meta: %v", err)
	}

	unitRates := map[string]shapePrice{}
	for _, p := range meta.Prices {
		if _, ok := shapeSet[p.ShapeName]; !ok {
			continue
		}
		unitRates[p.ShapeName] = p
	}
	for s := range shapeSet {
		if _, ok := unitRates[s]; !ok {
			fatalf("shape %q not found in %s", s, shapeMetaPath)
		}
	}

	prices := map[string]float64{}
	var shapeNames []string
	for s := range shapeSet {
		shapeNames = append(shapeNames, s)
	}
	sort.Strings(shapeNames)

	for _, shape := range shapeNames {
		rate := unitRates[shape]
		for _, c := range combos {
			key := fmt.Sprintf("%s.%do.%dg.1_1b", shape, c.ocpu, c.mem)
			prices[key] = hourly(rate, c.ocpu, c.mem)
		}
		if includeBase {
			prices[shape] = hourly(rate, base.ocpu, base.mem)
		}
	}

	out, err := json.MarshalIndent(prices, "", "  ")
	if err != nil {
		fatalf("marshal output: %v", err)
	}
	out = append(out, '\n')
	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		fatalf("write output: %v", err)
	}
	fmt.Printf("wrote %d prices to %s using %s\n", len(prices), outPath, shapeMetaPath)
}

func hourly(rate shapePrice, ocpu, mem int) float64 {
	// Using OCI Flex unit rates from KPO shape metadata.
	return float64(ocpu)*rate.OcpuUnitPrice + float64(mem)*rate.MemoryUnitPrice
}

func parseCombos(s string) ([]combo, error) {
	var out []combo
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		c, err := parseCombo(part)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid combos")
	}
	return out, nil
}

func parseCombo(s string) (combo, error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return combo{}, fmt.Errorf("invalid combo %q, expected OCPU:MEMORYGB", s)
	}
	ocpu, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return combo{}, fmt.Errorf("invalid OCPU in %q", s)
	}
	mem, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return combo{}, fmt.Errorf("invalid memory in %q", s)
	}
	if ocpu <= 0 || mem <= 0 {
		return combo{}, fmt.Errorf("combo values must be > 0 in %q", s)
	}
	return combo{ocpu: ocpu, mem: mem}, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
