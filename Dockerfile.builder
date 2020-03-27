FROM golang:1.14.1
RUN \
  apt-get update -qq && \
  apt-get install -qq -y build-essential autoconf automake autotools-dev
