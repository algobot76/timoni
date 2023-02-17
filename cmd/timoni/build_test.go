package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/fluxcd/pkg/ssa"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestBuild(t *testing.T) {
	modPath := "testdata/cs"

	t.Run("builds module with default values", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-instance", 5)
		namespace := rnd("my-namespace", 5)
		output, err := executeCommand(fmt.Sprintf(
			"build -n %s %s %s -p main -o yaml",
			namespace,
			name,
			modPath,
		))
		g.Expect(err).NotTo(HaveOccurred())

		objects, err := ssa.ReadObjects(strings.NewReader(output))
		g.Expect(err).NotTo(HaveOccurred())

		g.Expect(output).To(ContainSubstring("tcp://example.internal"))

		for _, o := range objects {
			g.Expect(o.GetKind()).To(BeEquivalentTo("ConfigMap"))
			g.Expect(o.GetName()).To(ContainSubstring(name))
			g.Expect(o.GetNamespace()).To(ContainSubstring(namespace))
		}
	})

	t.Run("builds module and outputs JSON", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-instance", 5)
		namespace := rnd("my-namespace", 5)
		output, err := executeCommand(fmt.Sprintf(
			"build -n %s %s %s -p main -o json",
			namespace,
			name,
			modPath,
		))
		g.Expect(err).NotTo(HaveOccurred())

		g.Expect(output).To(ContainSubstring("\"kind\": \"List\""))

		objects, err := ssa.ReadObjects(strings.NewReader(output))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(len(objects)).To(BeEquivalentTo(2))
	})

	t.Run("builds module with custom values", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-instance", 5)
		namespace := rnd("my-namespace", 5)
		output, err := executeCommand(fmt.Sprintf(
			"build -n %s %s %s -f %s -p main -o yaml",
			namespace,
			name,
			modPath,
			modPath+"-values/example.com.cue",
		))
		g.Expect(err).NotTo(HaveOccurred())

		g.Expect(output).To(ContainSubstring("tcp://example.com"))

		objects, err := ssa.ReadObjects(strings.NewReader(output))
		g.Expect(err).NotTo(HaveOccurred())

		g.Expect(len(objects)).To(BeEquivalentTo(2))
		for _, o := range objects {
			g.Expect(o.GetAnnotations()).To(HaveKeyWithValue("scope", "external"))
		}
	})

	t.Run("builds module with merged values", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-instance", 5)
		namespace := rnd("my-namespace", 5)
		output, err := executeCommand(fmt.Sprintf(
			"build -n %s %s %s -f %s -f %s -f %s -p main -o yaml",
			namespace,
			name,
			modPath,
			modPath+"-values/example.com.cue",
			modPath+"-values/example.io.cue",
			modPath+"-values/client-only.cue",
		))
		g.Expect(err).NotTo(HaveOccurred())

		objects, err := ssa.ReadObjects(strings.NewReader(output))
		g.Expect(err).NotTo(HaveOccurred())

		g.Expect(len(objects)).To(BeEquivalentTo(1))
		g.Expect(objects[0].GetName()).To(BeEquivalentTo(name + "-client"))
		g.Expect(objects[0].GetAnnotations()).To(HaveKeyWithValue("scope", "external"))

		val, _, err := unstructured.NestedString(objects[0].Object, "data", "server")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(val).To(BeEquivalentTo("tcp://example.io:9090"))
	})

	t.Run("fails to build with invalid values", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-instance", 5)
		namespace := rnd("my-namespace", 5)
		output, err := executeCommand(fmt.Sprintf(
			"build -n %s %s %s -f %s -p main -o yaml",
			namespace,
			name,
			modPath,
			modPath+"-values/invalid.cue",
		))
		g.Expect(output).To(BeEmpty())
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("client.enabled"))
	})

	t.Run("fails to build with undefined package", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-instance", 5)
		namespace := rnd("my-namespace", 5)
		output, err := executeCommand(fmt.Sprintf(
			"build -n %s %s %s -p test -o yaml",
			namespace,
			name,
			modPath,
		))
		g.Expect(output).To(BeEmpty())
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("cannot find package"))
	})
}
