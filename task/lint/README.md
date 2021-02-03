## Lint

This folder defines a lint pipeline using [github/super-linter](https://github.com/github/super-linter) to perform the linting of different programming languages in your git repository.

The [github/super-linter](https://github.com/github/super-linter) lint outputs `*.tap` files using the [TAP Protocol format](https://testanything.org/) which are then converted to comments on the Pull Request via [jx-tap](https://github.com/jenkins-x-plugins/jx-tap)

### Adding this pipeline to your project

If you are using [Jenkins X V3](https://jenkins-x.io/v3/about/) or using in-repo configuration with [Lighthouse](https://github.com/jenkins-x/lighthouse) you can add the linter pipeline to your project via:

* make sure you have [kpt](https://googlecontainertools.github.io/kpt/) on your `$PATH`
* run the following command

```bash
mkdir -p .lighthouse
kpt pkg get https://github.com/jenkins-x/jx3-pipeline-catalog.git/task/lint .lighthouse/lint
git add .lighthouse
git commit -a -m "chore: added linter"
```

