# Looker api credentials

the `build-lookml-datatest` step uses [spectacles CI](https://github.com/spectacles-ci/spectacles) to run looker data tests and sql tests. this requires credentials to authenticate to your pre-production looker instance

for example:

- update the LOOKER_BASE_URL value in your code repo `.lighthouse/pullrequest.yaml`
- [create an api user](https://docs.spectacles.dev/cli/guides/how-to-create-an-api-key)
- create a k8s/jenkins-x secret named `looker-sdk` containing the api credentials
- add/uncomment this `env` block in your code repo's `.lighthouse/pullrequest.yaml`

```yaml
taskSpec:
  metadata: {}
  stepTemplate:
    env:
      - name: LOOKER_BASE_URL
        value: https://looker-api.jx-staging.$YOUR_DOMAIN.com
      - name: LOOKER_CLIENT_ID
        valueFrom:
          secretKeyRef:
            name: looker-sdk
            key: client-id
      - name: LOOKER_CLIENT_SECRET
        valueFrom:
          secretKeyRef:
            name: looker-sdk
            key: client-secret
```

alternatively, you could inject looker credentials at run time using [banzaicloud-stable/vault-secrets-webhook](https://github.com/banzaicloud/bank-vaults/tree/master/charts/vault-secrets-webhook)

```yaml
taskSpec:
  metadata:
    annotations:
      vault.security.banzaicloud.io/vault-addr: "https://vault:8200"
      vault.security.banzaicloud.io/vault-role: default
      vault.security.banzaicloud.io/vault-tls-secret: vault-tls
  stepTemplate:
    env:
      - name: LOOKER_BASE_URL
        value: https://looker-api.jx-staging.$YOUR_DOMAIN.com
      - name: LOOKER_PROJECT
        value: $YOUR_LOOKER_PROJECT
      - name: LOOKER_CLIENT_ID
        value: vault:secret/data/looker#sdk_client_id
      - name: LOOKER_CLIENT_SECRET
        value: vault:secret/data/looker#sdk_client_secret
```
