package main

import (
	"go.uber.org/fx"

	"github.com/HuyDoan-2112/handle-massive-requests/internal/bootstrap"
)

func main() {
	fx.New(bootstrap.Module).Run()
}
