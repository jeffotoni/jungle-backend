#!/usr/bin/env sh
set -eu

awslocal sqs create-queue --queue-name jungle-wager >/dev/null
awslocal sqs create-queue --queue-name jungle-events >/dev/null
