#!/bin/bash

VERSION=1.14.2

set -ex

docker build -t quay.io/brankas/assetgen:$VERSION .
docker push quay.io/brankas/assetgen:$VERSION
