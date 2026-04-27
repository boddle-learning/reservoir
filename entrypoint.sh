#!/bin/sh

if [ ! -z ${SSM_PATH} ]; then
  awsenv
  source /ssm/.env
fi


exec "$@"