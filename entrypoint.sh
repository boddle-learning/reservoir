#!/bin/sh

if [ -n "${SSM_PATH}" ]; then
  awsenv
  # POSIX sh uses `.` for sourcing; `source` is a bash-ism that
  # isn't guaranteed in the container's /bin/sh (Alpine's busybox ash).
  . /ssm/.env
fi


exec "$@"