/*
Copyright 2019 The Kubernetes Authors.

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

package terraform

import (
	"bytes"
	"fmt"
	"strings"
	"regexp"

	"github.com/hashicorp/hcl/hcl/ast"
	hcl_printer "github.com/hashicorp/hcl/hcl/printer"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"

	"k8s.io/klog"
	"k8s.io/kops/pkg/featureflag"
)

func hcl2Print(node ast.Node) ([]byte, error) {
	var sanitizer astSanitizer
	sanitizer.visit(node)

	var b bytes.Buffer
	err := hcl_printer.Fprint(&b, node)
	if err != nil {
		return nil, fmt.Errorf("error writing HCL: %v", err)
	}
	s := b.String()

	// Remove extra whitespace...
	s = strings.Replace(s, "\n\n", "\n", -1)

	// ...but leave whitespace between resources
	s = strings.Replace(s, "}\nresource", "}\n\nresource", -1)

	// Workaround HCL insanity #6359: quotes are _not_ escaped in quotes (huh?)
	// This hits the file function
	s = strings.Replace(s, "(\\\"", "(\"", -1)
	s = strings.Replace(s, "\\\")", "\")", -1)

	// We don't need to escape > or <
	s = strings.Replace(s, "\\u003c", "<", -1)
	s = strings.Replace(s, "\\u003e", ">", -1)

	// terraform 0.12 - change block assignments to block definitions
	s = strings.Replace(s, " = {", " {", -1)

	// Qoute any unquoted tag Map keys 
	//   Specifically => Name, KubernetesCluster, SubnetType
	//   ([^"])(Name|KubernetesCluster|SubnetType)([^"])(\s*)=
	re := regexp.MustCompile(`(?P<1>[^"])(?P<key>Name|KubernetesCluster|SubnetType)(?P<2>[^"])(?P<3>\s*)=`)
	s = re.ReplaceAllString(s, `${1}"${key}"${2}${3}=`))

	f, diagnostics := hclwrite.ParseConfig([]byte(s), "kubernetes.tf", hcl.Pos{})
	if diagnostics.HasErrors() {
		//return nil, fmt.Errorf("error parsing terraform hcl: %v", diagnostics)
		fmt.Printf("error parsing terraform hcl: %v", diagnostics)
		return []byte(s), nil
	}

	if featureflag.SkipTerraformFormat.Enabled() {
		klog.Infof("feature-flag SkipTerraformFormat was set; skipping terraform format")
		return f.Bytes(), nil
	}

	// Apply Terraform style (alignment etc.)
	formatted := hclwrite.Format(f.Bytes())
	return formatted, nil
}
