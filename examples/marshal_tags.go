package main

import (
	"fmt"

	"github.com/oarkflow/fastjson"
)

type App struct {
	LicenseKey string `json:"license_key"`
	Name       string `json:"name"`
}

type Payment struct {
	CardNumber string `json:"card_number"`
	CVV        string `json:"cvv"`
}

type Tenant struct {
	PrivateNote string `json:"private_note"`
}

type Config struct {
	App     App     `json:"app"`
	Payment Payment `json:"payment"`
	Tenant  Tenant `json:"tenant"`
}

func main() {
	cfg := Config{
		App:     App{LicenseKey: "<hidden>", Name: "Workflow Enterprise"},
		Payment: Payment{CardNumber: "<hidden>", CVV: "<hidden>"},
		Tenant:  Tenant{PrivateNote: "<hidden>"},
	}

	b, err := fastjson.MarshalIndent(cfg, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
}
