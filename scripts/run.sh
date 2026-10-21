#!/usr/bin/env bash

go run -ldflags "-X 'github.com/dont-be-evil-company/nvpm-client/cmd/nvpm.VERSION=${VERSION}'" main.go
