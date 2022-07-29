#!/usr/bin/env bash

printf "READY\n";

while read -r line; do
  echo "Processing Event: $line" >&2;
  kill -3 "$(cat "/var/run/supervisord.pid")"
done < /dev/stdin
