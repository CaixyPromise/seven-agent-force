#!/bin/sh
set -eu
test "$(cat value.txt)" = good
