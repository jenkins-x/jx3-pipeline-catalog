## Lint

This folder defines a lint pipeline using [github/super-linter](https://github.com/github/super-linter) to perform the linting of different programming languages in your git repository.

The [github/super-linter](https://github.com/github/super-linter) lint outputs `*.tap` files using the [TAP Protocol format](https://testanything.org/) which are then converted to comments on the Pull Request via [jx-tap](https://github.com/jenkins-x-plugins/jx-tap)
