#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

package_path="/Users/yuanmh/Desktop/code/k8s-crds-clientsets"


/root/go/pkg/mod/k8s.io/code-generator@v0.26.1/generate-groups.sh "client,informer,lister" \
../pkg/client \
../pkg/apis \
  reservation:v1alpha1 \
  --go-header-file ../hack/boilerplate.go.txt
