#!/bin/bash

VERSION=1.14.1

set -ex

docker build \
  -t registry.brankas.dev/assetgen/builder:$VERSION \
  -f Dockerfile.builder \
  .

docker push registry.brankas.dev/assetgen/builder:$VERSION
