# service-level-operator-sloth-migrator

A simple CLI that will migrate [service-level-operator] CRs to [sloth] CRs.

## Getting started

These the steps:

- Get [service-level-operator] SLOs from Kubernetes using `kubectl`.
- Use this CLI to migrate them.
- Apply new sloth CRs using [sloth] CLI or Kubernetes controller mode.

Lets see an example:

```bash
# Get service-level-operator SLOs.
kubect get --all-namespaces -o yaml > ./slos.yaml

# Migrate to sloth.
mkdir ./sloth-specs
go run ./  --slos ./slos.yaml --out ./sloth-specs

# Check sloth specs by generating prometheus-operator rules CRs.
mkdir ./prom-specs
for f in ./out/*; do sloth generate -i ${f} -o ./prom-specs/$(basename ${f}); done
```

## Advanced options

- `--ignore-disable`: If used, it will not migrate the SLOs that have `disable: true`.

[service-level-operator]: https://github.com/spotahome/service-level-operator
[sloth]: https://github.com/slok/sloth
