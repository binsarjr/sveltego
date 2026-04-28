#!/usr/bin/env bash
set -euo pipefail
git config core.hooksPath .githooks
chmod +x .githooks/*
echo "Hooks installed (core.hooksPath=.githooks)."
