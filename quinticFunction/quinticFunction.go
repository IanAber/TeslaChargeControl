package quinticFunction

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
)

type QuinticFunction struct {
	Max struct {
		A float64 `json:"a"`
		B float64 `json:"b"`
		C float64 `json:"c"`
		D float64 `json:"d"`
		E float64 `json:"e"`
		F float64 `json:"f"`
	} `json:"max"`
	Min struct {
		A float64 `json:"a"`
		B float64 `json:"b"`
		C float64 `json:"c"`
		D float64 `json:"d"`
		E float64 `json:"e"`
		F float64 `json:"f"`
	} `json:"min"`
}

func (eq *QuinticFunction) LoadConstants(filename string) error {
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	err = json.Unmarshal(file, eq)
	if err != nil {
		return err
	}
	if (eq.Min.A == 0.0) && (eq.Max.A == 0.0) && (eq.Min.B == 0.00) && (eq.Max.B == 0.0) {
		fmt.Print("failed to get the parameters")
		err = errors.New("failed to get the parameters")
	}
	return err
}

func (eq *QuinticFunction) Eval(x float32) (max float32, min float32) {
	x64 := float64(x)
	max = float32((0 - eq.Max.A) + (eq.Max.B * x64) - (eq.Max.C * math.Pow(x64, 2)) + (eq.Max.D * math.Pow(x64, 3)) - (eq.Max.E * math.Pow(x64, 4)) + (eq.Max.F * math.Pow(x64, 5)))
	min = float32((0 - eq.Min.A) + (eq.Min.B * x64) - (eq.Min.C * math.Pow(x64, 2)) + (eq.Min.D * math.Pow(x64, 3)) - (eq.Min.E * math.Pow(x64, 4)) + (eq.Min.F * math.Pow(x64, 5)))
	return max, min
}
