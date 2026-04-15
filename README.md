# oke-node-viewer

`oke-node-viewer` is a terminal UI for visualizing Kubernetes node allocatable capacity versus scheduled pod requests.

## Highlights

- Watches nodes and pods from the selected kubeconfig context
- Displays per-node and cluster-level resource pressure
- Uses static pricing from JSON (embedded defaults + optional override file)

## Build

```bash
make build
```

## Run

```bash
./oke-node-viewer --kubeconfig ~/.kube/config --context <your-oke-context>
```

## Pricing

By default, the binary embeds a static pricing map at:

- `pkg/pricing/static_prices.json`
- You can refresh this file from OCI list pricing with `make pricing-update`
- You can refresh this file from KPO shape metadata with `make pricing-update-shape-meta`
- Update `pkg/pricing/oci_part_numbers.json` with valid OCI part numbers first

### OCI Flex Pricing Notes

For OCI Flex shapes, Oracle pricing is exposed as separate line items:

- OCPU hourly price
- Memory (GB) hourly price

There is typically not a single all-in hourly SKU per Flex shape in the list-pricing feed.
For accurate per-node pricing, compute:

`hourly = (ocpus * ocpu_rate) + (memory_gb * memory_rate)`

and provide the final prices via `--pricing-file` keyed by the exact Kubernetes instance-type
label (for example `VM.Standard.E3.Flex.4o.16g.1_1b`).

### About `make pricing-update`

`make pricing-update` uses `hack/fetch_oci_pricing.go`, which currently expects a single
`shape -> partNumber` mapping entry and writes one static price per key. That is best treated
as a convenience workflow for simple/static mappings.

For Flex shape accuracy, prefer generating a per-permutation pricing file and passing it with
`--pricing-file`.

Recommended workflow when you have the local KPO repo:

```bash
make pricing-update-shape-meta
```

By default this reads `~/karpenter-provider-oci/chart/config/oci-shape-meta.json` and computes Flex prices as:

`hourly = (ocpus * ocpuUnitPrice) + (memory_gb * memoryUnitPrice)`

You can provide your own file:

```bash
./oke-node-viewer --pricing-file ./my-prices.json
```

File format:

```json
{
  "VM.Standard.E4.Flex": 0.08,
  "VM.Standard.E5.Flex": 0.09
}
```

Per-permutation example (recommended for Flex):

```json
{
  "VM.Standard.E3.Flex.1o.4g.1_1b": 0.031,
  "VM.Standard.E3.Flex.2o.8g.1_1b": 0.062,
  "VM.Standard.E3.Flex.4o.16g.1_1b": 0.124,
  "VM.Standard.E3.Flex.8o.32g.1_1b": 0.248,
  "VM.Standard.E4.Flex.1o.4g.1_1b": 0.031,
  "VM.Standard.E4.Flex.2o.8g.1_1b": 0.062,
  "VM.Standard.E4.Flex.4o.16g.1_1b": 0.124,
  "VM.Standard.E4.Flex.8o.32g.1_1b": 0.248,
  "VM.Standard.E5.Flex.1o.4g.1_1b": 0.038,
  "VM.Standard.E5.Flex.2o.8g.1_1b": 0.076,
  "VM.Standard.E5.Flex.4o.16g.1_1b": 0.152,
  "VM.Standard.E5.Flex.8o.32g.1_1b": 0.304
}
```

You can also set per-node override labels:

- `oke-node-viewer/instance-price`

## Config file

Defaults can be set in:

- `~/.oke-node-viewer`

Format:

```text
node-selector=karpenter.sh/nodepool
resources=cpu,memory
extra-labels=topology.kubernetes.io/zone,karpenter.sh/nodepool
node-sort=creation=asc
style=#2E91D2,#ffff00,#D55E00
pricing-file=/path/to/prices.json
```

## Refreshing Prices From OCI

The repository includes `hack/fetch_oci_pricing.go`, which calls OCI list pricing and writes `pkg/pricing/static_prices.json`.

1. Populate `pkg/pricing/oci_part_numbers.json` with real `shape -> partNumber` values.
2. Run:

```bash
make pricing-update
```

You can also run the script directly:

```bash
go run ./hack/fetch_oci_pricing.go \
  --endpoint https://apexapps.oracle.com/pls/apex/cetools/api/v1/products/ \
  --currency USD \
  --mapping ./pkg/pricing/oci_part_numbers.json \
  --out ./pkg/pricing/static_prices.json
```

For shape-meta generation:

```bash
go run ./hack/generate_oci_flex_prices_from_shape_meta.go \
  --shapes VM.Standard.E3.Flex,VM.Standard.E4.Flex,VM.Standard.E5.Flex \
  --combos 1:4,2:8,4:16,8:32 \
  --out ./pkg/pricing/static_prices.json
```

If your KPO repo is somewhere else, pass `--shape-meta <path>`.
