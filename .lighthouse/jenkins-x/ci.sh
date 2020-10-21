#!/usr/bin/env bash
set -e

declare -a repos=(
  "nodedemo"
)

CURDIR=$(pwd)
export PROJECT_DIR=test-projects
rm -rf $PROJECT_DIR
mkdir -p $PROJECT_DIR

for r in "${repos[@]}"
do
  echo "upgrading repository https://github.com/jenkins-x-labs-bdd-tests/$r"

  cd $PROJECT_DIR
  git clone https://github.com/jenkins-x-labs-bdd-tests/$r.git
  cd $r

  echo "removing old build pack"
  rm -rf .lighthouse/ jenkins-x.yml charts preview Dockerfile
  echo "recreating the pipeline... in dir $(pwd)"

  # lets regenerate the build pack...
  jx project import --no-dev-pr --dry-run --batch-mode --dir $(pwd)  --pipeline-catalog-dir $CURDIR/../../packs

  git add * || true
  git commit -a -m "chore: upgrade pipeline library" || true
  git push || true

  echo "updated the pipeline library for $r"
done


echo "finished"

