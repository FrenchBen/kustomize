// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package target_test

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kustomize/api/ifc"
	. "sigs.k8s.io/kustomize/api/internal/target"
	"sigs.k8s.io/kustomize/api/loader"
	"sigs.k8s.io/kustomize/api/provider"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
	kusttest_test "sigs.k8s.io/kustomize/api/testutils/kusttest"
	"sigs.k8s.io/kustomize/api/types"
)

// KustTarget is primarily tested in the krusty package with
// high level tests.

func TestLoadKustFile(t *testing.T) {
	for name, test := range map[string]struct {
		fileNames            []string
		kustFileName, errMsg string
	}{
		"missing": {
			fileNames: []string{"kustomization"},
			errMsg:    `unable to find one of 'kustomization.yaml', 'kustomization.yml' or 'Kustomization' in directory '/'`,
		},
		"multiple": {
			fileNames: []string{"kustomization.yaml", "Kustomization"},
			errMsg: `Found multiple kustomization files under: /
`,
		},
		"valid": {
			fileNames:    []string{"kustomization.yml", "kust"},
			kustFileName: "kustomization.yml",
		},
	} {
		t.Run(name, func(t *testing.T) {
			th := kusttest_test.MakeHarness(t)
			fSys := th.GetFSys()
			for _, file := range test.fileNames {
				require.NoError(t, fSys.WriteFile(file, []byte(fmt.Sprintf("namePrefix: test-%s", file))))
			}

			content, fileName, err := LoadKustFile(loader.NewFileLoaderAtCwd(fSys))
			if test.kustFileName != "" {
				require.NoError(t, err)
				require.Equal(t, fmt.Sprintf("namePrefix: test-%s", test.kustFileName), string(content))
				require.Equal(t, test.kustFileName, fileName)
			} else {
				require.EqualError(t, err, test.errMsg)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	th := kusttest_test.MakeHarness(t)
	expectedTypeMeta := types.TypeMeta{
		APIVersion: "kustomize.config.k8s.io/v1beta1",
		Kind:       "Kustomization",
	}

	testCases := map[string]struct {
		errContains string
		content     string
		k           types.Kustomization
	}{
		"empty": {
			// no content
			k: types.Kustomization{
				TypeMeta: expectedTypeMeta,
			},
			errContains: "kustomization.yaml is empty",
		},
		"nonsenseLatin": {
			errContains: "found a tab character that violates indentation",
			content: `
		Lorem ipsum dolor sit amet, consectetur
		adipiscing elit, sed do eiusmod tempor
		incididunt ut labore et dolore magna aliqua.
		Ut enim ad minim veniam, quis nostrud
		exercitation ullamco laboris nisi ut
		aliquip ex ea commodo consequat.
		`,
		},
		"simple": {
			content: `
commonLabels:
  app: nginx
`,
			k: types.Kustomization{
				TypeMeta:     expectedTypeMeta,
				CommonLabels: map[string]string{"app": "nginx"},
			},
		},
		"commented": {
			content: `
# Licensed to the Blah Blah Software Foundation
# ...
# yada yada yada.

commonLabels:
 app: nginx
`,
			k: types.Kustomization{
				TypeMeta:     expectedTypeMeta,
				CommonLabels: map[string]string{"app": "nginx"},
			},
		},
	}

	kt := makeKustTargetWithRf(
		t, th.GetFSys(), "/", provider.NewDefaultDepProvider())
	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			th.WriteK("/", tc.content)
			err := kt.Load()
			if tc.errContains != "" {
				require.NotNilf(t, err, "expected error containing: `%s`", tc.errContains)
				require.Contains(t, err.Error(), tc.errContains)
			} else {
				require.Nilf(t, err, "got error: %v", err)
				k := kt.Kustomization()
				require.Condition(t, func() bool {
					return reflect.DeepEqual(tc.k, k)
				}, "expected %v, got %v", tc.k, k)
			}
		})
	}
}

func TestMakeCustomizedResMap(t *testing.T) {
	th := kusttest_test.MakeHarness(t)
	th.WriteK("/whatever", `namePrefix: foo-
nameSuffix: -bar
namespace: ns1
commonLabels:
  app: nginx
commonAnnotations:
  note: This is a test annotation
resources:
  - deployment.yaml
  - namespace.yaml
generatorOptions:
  disableNameSuffixHash: false
configMapGenerator:
- name: literalConfigMap
  literals:
  - DB_USERNAME=admin
  - DB_PASSWORD=somepw
secretGenerator:
- name: secret
  literals:
    - DB_USERNAME=admin
    - DB_PASSWORD=somepw
  type: Opaque
patchesJson6902:
- target:
    group: apps
    version: v1
    kind: Deployment
    name: dply1
  path: jsonpatch.json
`)
	th.WriteF("/whatever/deployment.yaml", `
apiVersion: apps/v1
metadata:
  name: dply1
kind: Deployment
`)
	th.WriteF("/whatever/namespace.yaml", `
apiVersion: v1
kind: Namespace
metadata:
  name: ns1
`)
	th.WriteF("/whatever/jsonpatch.json", `[
    {"op": "add", "path": "/spec/replica", "value": "3"}
]`)

	pvd := provider.NewDefaultDepProvider()
	resFactory := pvd.GetResourceFactory()

	resources := []*resource.Resource{
		resFactory.FromMapWithName("dply1", map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "foo-dply1-bar",
				"namespace": "ns1",
				"labels": map[string]interface{}{
					"app": "nginx",
				},
				"annotations": map[string]interface{}{
					"note": "This is a test annotation",
				},
			},
			"spec": map[string]interface{}{
				"replica": "3",
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app": "nginx",
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"note": "This is a test annotation",
						},
						"labels": map[string]interface{}{
							"app": "nginx",
						},
					},
				},
			},
		}),
		resFactory.FromMapWithName("ns1", map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "ns1",
				"labels": map[string]interface{}{
					"app": "nginx",
				},
				"annotations": map[string]interface{}{
					"note": "This is a test annotation",
				},
			},
		}),
		resFactory.FromMapWithName("literalConfigMap",
			map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "foo-literalConfigMap-bar-g5f6t456f5",
					"namespace": "ns1",
					"labels": map[string]interface{}{
						"app": "nginx",
					},
					"annotations": map[string]interface{}{
						"note": "This is a test annotation",
					},
				},
				"data": map[string]interface{}{
					"DB_USERNAME": "admin",
					"DB_PASSWORD": "somepw",
				},
			}),
		resFactory.FromMapWithName("secret",
			map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      "foo-secret-bar-82c2g5f8f6",
					"namespace": "ns1",
					"labels": map[string]interface{}{
						"app": "nginx",
					},
					"annotations": map[string]interface{}{
						"note": "This is a test annotation",
					},
				},
				"type": ifc.SecretTypeOpaque,
				"data": map[string]interface{}{
					"DB_USERNAME": base64.StdEncoding.EncodeToString([]byte("admin")),
					"DB_PASSWORD": base64.StdEncoding.EncodeToString([]byte("somepw")),
				},
			}),
	}

	expected := resmap.New()
	for _, r := range resources {
		if err := expected.Append(r); err != nil {
			t.Fatalf("unexpected error %v", err)
		}
	}
	expected.RemoveBuildAnnotations()
	expYaml, err := expected.AsYaml()
	assert.NoError(t, err)

	kt := makeKustTargetWithRf(t, th.GetFSys(), "/whatever", pvd)
	assert.NoError(t, kt.Load())
	actual, err := kt.MakeCustomizedResMap()
	assert.NoError(t, err)
	actual.RemoveBuildAnnotations()
	actYaml, err := actual.AsYaml()
	assert.NoError(t, err)
	assert.Equal(t, string(expYaml), string(actYaml))
}

func TestConfigurationsOverrideDefault(t *testing.T) {
	th := kusttest_test.MakeHarness(t)
	th.WriteK("/merge-config", `namePrefix: foo-
nameSuffix: -bar
namespace: ns1
resources:
  - deployment.yaml
  - config.yaml
  - secret.yaml
configurations:
  - name-prefix-rules.yaml
  - name-suffix-rules.yaml
`)
	th.WriteF("/merge-config/name-prefix-rules.yaml", `
namePrefix:
- path: metadata/name
  apiVersion: v1
  kind: Deployment
- path: metadata/name
  apiVersion: v1
  kind: Secret
`)
	th.WriteF("/merge-config/name-suffix-rules.yaml", `
nameSuffix:
- path: metadata/name
  apiVersion: v1
  kind: ConfigMap
- path: metadata/name
  apiVersion: v1
  kind: Deployment
`)
	th.WriteF("/merge-config/deployment.yaml", `
apiVersion: apps/v1
metadata:
  name: deployment1
kind: Deployment
`)
	th.WriteF("/merge-config/config.yaml", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: config
`)
	th.WriteF("/merge-config/secret.yaml", `
apiVersion: v1
kind: Secret
metadata:
  name: secret
`)

	pvd := provider.NewDefaultDepProvider()
	resFactory := pvd.GetResourceFactory()

	resources := []*resource.Resource{
		resFactory.FromMapWithName("deployment1", map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "foo-deployment1-bar",
				"namespace": "ns1",
			},
		}), resFactory.FromMapWithName("config", map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "config-bar",
				"namespace": "ns1",
			},
		}), resFactory.FromMapWithName("secret", map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "foo-secret",
				"namespace": "ns1",
			},
		}),
	}

	expected := resmap.New()
	for _, r := range resources {
		err := expected.Append(r)
		require.NoError(t, err)
	}
	expected.RemoveBuildAnnotations()
	expYaml, err := expected.AsYaml()
	require.NoError(t, err)

	kt := makeKustTargetWithRf(t, th.GetFSys(), "/merge-config", pvd)
	require.NoError(t, kt.Load())
	actual, err := kt.MakeCustomizedResMap()
	require.NoError(t, err)
	actual.RemoveBuildAnnotations()
	actYaml, err := actual.AsYaml()
	require.NoError(t, err)
	require.Equal(t, string(expYaml), string(actYaml))
}

func TestDuplicateExternalGeneratorsForbidden(t *testing.T) {
	th := kusttest_test.MakeHarness(t)
	th.WriteK("/generator", `generators:
- |-
  apiVersion: generators.example/v1
  kind: ManifestGenerator
  metadata:
    name: ManifestGenerator
    annotations:
      config.kubernetes.io/function: |
        container:
          image: ManifestGenerator:latest
  spec:
    image: 'someimage:12345'
    configPath: config.json
- |-
  apiVersion: generators.example/v1
  kind: ManifestGenerator
  metadata:
    name: ManifestGenerator
    annotations:
      config.kubernetes.io/function: |
        container:
          image: ManifestGenerator:latest
  spec:
    image: 'someimage:12345'
    configPath: another_config.json
`)
	_, err := makeAndLoadKustTarget(t, th.GetFSys(), "/generator").AccumulateTarget()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "may not add resource with an already registered id: ManifestGenerator.v1.generators.example/ManifestGenerator")
}

func TestDuplicateExternalTransformersForbidden(t *testing.T) {
	th := kusttest_test.MakeHarness(t)
	th.WriteK("/transformer", `transformers:
- |-
  apiVersion: transformers.example.co/v1
  kind: ValueAnnotator
  metadata:
    name: notImportantHere
    annotations:
      config.kubernetes.io/function: |
        container:
          image: example.docker.com/my-functions/valueannotator:1.0.0
  value: 'pass'
- |-
  apiVersion: transformers.example.co/v1
  kind: ValueAnnotator
  metadata:
    name: notImportantHere
    annotations:
      config.kubernetes.io/function: |
        container:
          image: example.docker.com/my-functions/valueannotator:1.0.0
  value: 'fail'
`)
	_, err := makeAndLoadKustTarget(t, th.GetFSys(), "/transformer").AccumulateTarget()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "may not add resource with an already registered id: ValueAnnotator.v1.transformers.example.co/notImportantHere")
}
