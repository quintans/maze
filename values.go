package maze

import (
	"strconv"
	"time"
)

const ISO8601 = "2006-01-02T15:04:05.999999999Z0700"

type Values map[string][]string

func (p Values) AsStrings(k string) []string {
	if p == nil {
		return []string{}
	}
	var v = p[k]
	if v == nil {
		return []string{}
	}
	return v
}

func (p Values) AsString(k string) string {
	var v = p.AsStrings(k)
	if len(v) > 0 {
		return v[0]
	}

	return ""
}

func (p Values) AsBools(k string) []bool {
	var arr = make([]bool, 0)
	for _, s := range p.AsStrings(k) {
		if v, err := strconv.ParseBool(s); err == nil {
			arr = append(arr, v)
		}
	}
	return arr
}

func (p Values) AsBool(k string) bool {
	if v, err := strconv.ParseBool(p.AsString(k)); err == nil {
		return v
	}
	return false
}

func (p Values) AsFloats(k string) []float64 {
	var arr = make([]float64, 0)
	for _, s := range p.AsStrings(k) {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			arr = append(arr, v)
		}
	}
	return arr
}

func (p Values) AsFloat(k string) float64 {
	if v, err := strconv.ParseFloat(p.AsString(k), 64); err == nil {
		return v
	}
	return 0
}

func (p Values) AsInts(k string) []int64 {
	var arr = make([]int64, 0)
	for _, s := range p.AsStrings(k) {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			arr = append(arr, v)
		}
	}
	return arr
}

func (p Values) AsInt(k string) int64 {
	if v, err := strconv.ParseInt(p.AsString(k), 10, 64); err == nil {
		return v
	}
	return 0
}

func (p Values) AsUints(k string) []uint64 {
	var arr = make([]uint64, 0)
	for _, s := range p.AsStrings(k) {
		if v, err := strconv.ParseUint(s, 10, 64); err == nil {
			arr = append(arr, v)
		}
	}
	return arr
}

func (p Values) AsUint(k string) uint64 {
	if v, err := strconv.ParseUint(p.AsString(k), 10, 64); err == nil {
		return v
	}
	return 0
}

func (p Values) AsTimes(k string) []time.Time {
	var arr = make([]time.Time, 0)
	for _, s := range p.AsStrings(k) {
		if v, err := time.Parse(ISO8601, s); err == nil {
			arr = append(arr, v)
		}
	}
	return arr
}

func (p Values) AsTime(k string) time.Time {
	if v, err := time.Parse(ISO8601, p.AsString(k)); err == nil {
		return v
	}
	return time.Time{}
}
