#!/usr/bin/env sh
for chart in packs/*; do
  echo linting chart "$chart"
  jx gitops helm build -c "$chart"
done