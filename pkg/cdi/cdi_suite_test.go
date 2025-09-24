package cdi_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCDI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CDI Suite")
}
