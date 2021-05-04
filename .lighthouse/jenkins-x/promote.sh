#!/bin/sh

echo "promoting, restart lighthouse pods"
kubectl get pods -n jx | grep lighthouse- | awk '{print $1}' | xargs kubectl delete pod -n jx
