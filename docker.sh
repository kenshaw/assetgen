#!/bin/bash

VERSION=1.15.2

set -ex

docker build -t kenshaw/assetgen:$VERSION .
docker push kenshaw/assetgen:$VERSION
