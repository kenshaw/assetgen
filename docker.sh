#!/bin/bash

VERSION=1.15.4

set -ex

docker build -t kenshaw/assetgen:$VERSION .
docker push kenshaw/assetgen:$VERSION
