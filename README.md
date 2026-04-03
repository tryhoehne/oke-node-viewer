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
