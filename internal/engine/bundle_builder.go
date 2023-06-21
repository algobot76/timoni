/*
Copyright 2023 Stefan Prodan

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package engine

import (
	"fmt"
	"os"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/encoding/yaml"
	cp "github.com/otiai10/copy"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// BundleBuilder compiles CUE definitions to Go Bundle objects.
type BundleBuilder struct {
	ctx      *cue.Context
	files    []string
	injector *Injector
}

type Bundle struct {
	Name      string
	Instances []BundleInstance
}

type BundleInstance struct {
	Bundle    string
	Name      string
	Namespace string
	Module    apiv1.ModuleReference
	Values    cue.Value
}

// NewBundleBuilder creates a BundleBuilder for the given module and package.
func NewBundleBuilder(ctx *cue.Context, files []string) *BundleBuilder {
	if ctx == nil {
		ctx = cuecontext.New()
	}
	b := &BundleBuilder{
		ctx:      ctx,
		files:    files,
		injector: NewInjector(ctx),
	}
	return b
}

// InitWorkspace copies the bundle definitions to the specified workspace,
// sets the bundle schema, and then it injects values based on @timoni() attributes.
// A workspace must be initialised before calling Build.
func (b *BundleBuilder) InitWorkspace(workspace string) error {
	var files []string
	for i, file := range b.files {
		_, fn := filepath.Split(file)
		dstFile := filepath.Join(workspace, fmt.Sprintf("%v.%s", i, fn))
		files = append(files, dstFile)
		if err := cp.Copy(file, dstFile); err != nil {
			return err
		}
	}

	for _, f := range files {
		_, fn := filepath.Split(f)
		data, err := b.injector.Inject(f)
		if err != nil {
			return fmt.Errorf("failed to inject %s: %w", fn, err)
		}

		if err := os.WriteFile(f, data, os.ModePerm); err != nil {
			return fmt.Errorf("failed to inject %s: %w", fn, err)
		}
	}

	schemaFile := filepath.Join(workspace, fmt.Sprintf("%v.schema.cue", len(b.files)+1))
	files = append(files, schemaFile)
	if err := os.WriteFile(schemaFile, []byte(apiv1.BundleSchema), os.ModePerm); err != nil {
		return err
	}

	b.files = files
	return nil
}

// Build builds a CUE instance for the specified files and returns the CUE value.
// A workspace must be initialised with InitWorkspace before calling this function.
func (b *BundleBuilder) Build() (cue.Value, error) {
	var value cue.Value
	cfg := &load.Config{
		Package:   "_",
		DataFiles: true,
	}

	ix := load.Instances(b.files, cfg)
	if len(ix) == 0 {
		return value, fmt.Errorf("no instances found")
	}

	inst := ix[0]
	if inst.Err != nil {
		return value, fmt.Errorf("instance error: %w", inst.Err)
	}

	v := b.ctx.BuildInstance(inst)
	for _, f := range inst.OrphanedFiles {
		if f.Encoding == build.YAML {
			a, err := yaml.Extract(f.Filename, f.Source)
			if err != nil {
				return value, err
			}
			v = v.Unify(b.ctx.BuildFile(a))
		}
	}
	if v.Err() != nil {
		return value, v.Err()
	}

	if err := v.Validate(cue.Concrete(true)); err != nil {
		return value, err
	}

	return v, nil
}

// GetBundle returns a Bundle from the bundle CUE value.
func (b *BundleBuilder) GetBundle(v cue.Value) (*Bundle, error) {
	bundleNameValue := v.LookupPath(cue.ParsePath(apiv1.BundleName.String()))
	bundleName, err := bundleNameValue.String()
	if err != nil {
		return nil, fmt.Errorf("lookup %s failed: %w", apiv1.BundleName.String(), bundleNameValue.Err())
	}

	instances := v.LookupPath(cue.ParsePath(apiv1.BundleInstancesSelector.String()))
	if instances.Err() != nil {
		return nil, fmt.Errorf("lookup %s failed: %w", apiv1.BundleInstancesSelector.String(), instances.Err())
	}

	var list []BundleInstance
	iter, err := instances.Fields(cue.Concrete(true))
	if err != nil {
		return nil, err
	}

	for iter.Next() {
		name := iter.Selector().String()
		expr := iter.Value()

		vURL := expr.LookupPath(cue.ParsePath(apiv1.BundleModuleURLSelector.String()))
		url, _ := vURL.String()

		vDigest := expr.LookupPath(cue.ParsePath(apiv1.BundleModuleDigestSelector.String()))
		digest, _ := vDigest.String()

		vVersion := expr.LookupPath(cue.ParsePath(apiv1.BundleModuleVersionSelector.String()))
		version, _ := vVersion.String()

		vNamespace := expr.LookupPath(cue.ParsePath(apiv1.BundleNamespaceSelector.String()))
		namespace, _ := vNamespace.String()

		values := expr.LookupPath(cue.ParsePath(apiv1.BundleValuesSelector.String()))

		list = append(list, BundleInstance{
			Bundle:    bundleName,
			Name:      name,
			Namespace: namespace,
			Module: apiv1.ModuleReference{
				Repository: url,
				Version:    version,
				Digest:     digest,
			},
			Values: values,
		})
	}

	return &Bundle{
		Name:      bundleName,
		Instances: list,
	}, nil
}
