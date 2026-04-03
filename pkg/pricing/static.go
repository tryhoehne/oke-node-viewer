package pricing

import (
	_ "embed"
	"encoding/json"
	"log"
	"os"
	"strings"

	"github.com/treyhoehne/oke-node-viewer/pkg/model"
)

//go:embed static_prices.json
var embeddedStaticPrices []byte

type staticPricingProvider struct {
	prices map[string]float64
}

func NewStaticPricingProvider(pricingFile string) Provider {
	prices := map[string]float64{}

	if err := json.Unmarshal(embeddedStaticPrices, &prices); err != nil {
		log.Printf("unable to parse embedded static prices: %v", err)
	}

	if pricingFile != "" {
		override, err := loadStaticPricesFile(pricingFile)
		if err != nil {
			log.Printf("unable to read pricing file %q: %v", pricingFile, err)
		} else {
			for k, v := range override {
				prices[k] = v
			}
		}
	}

	return &staticPricingProvider{prices: prices}
}

func (p *staticPricingProvider) NodePrice(n *model.Node) (float64, bool) {
	price, ok := p.prices[strings.TrimSpace(n.InstanceType())]
	return price, ok
}

func (p *staticPricingProvider) OnUpdate(_ func()) {}

func loadStaticPricesFile(path string) (map[string]float64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	parsed := map[string]float64{}
	if err := json.Unmarshal(b, &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}
