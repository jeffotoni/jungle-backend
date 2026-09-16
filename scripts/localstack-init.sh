#!/usr/bin/env sh
set -eu

DLQ_URL=$(awslocal sqs create-queue \
	  --queue-name wager-transactions-dlq.fifo \
	    --attributes FifoQueue=true,ContentBasedDeduplication=true \
	      --query QueueUrl \
	        --output text)

DLQ_ARN=$(awslocal sqs get-queue-attributes \
	  --queue-url "$DLQ_URL" \
	    --attribute-names QueueArn \
	      --query 'Attributes.QueueArn' \
	        --output text)

MAIN_URL=$(awslocal sqs create-queue \
	  --queue-name wager-transactions.fifo \
	    --attributes FifoQueue=true,ContentBasedDeduplication=true \
	      --query QueueUrl \
	        --output text)

awslocal sqs set-queue-attributes \
	  --queue-url "$MAIN_URL" \
	    --attributes "{\"RedrivePolicy\":\"{\\\"deadLetterTargetArn\\\":\\\"$DLQ_ARN\\\",\\\"maxReceiveCount\\\":\\\"5\\\"}\"}"

awslocal sqs create-queue --queue-name jungle-wager >/dev/null
awslocal sqs create-queue --queue-name jungle-events >/dev/null
