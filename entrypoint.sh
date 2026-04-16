#!/bin/bash

set -e 

if [[ "${SSM_PATH}x" != "x" ]]; then
  awsenv && eval $(cat /ssm/.env)
fi

set -o errexit
set -o pipefail