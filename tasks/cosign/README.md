Tekton task for using https://github.com/sigstore/cosign

To use this task you will need a Kubernetes secret like so..

```yaml
apiVersion: v1
stringData:
  cosign.key: |-
    -----BEGIN ENCRYPTED COSIGN PRIVATE KEY-----
    abc123....
    -----END ENCRYPTED COSIGN PRIVATE KEY-----
  password: foo
kind: Secret
metadata:
  name: cosign
type: Opaque
```