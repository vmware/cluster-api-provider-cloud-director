/*
<<<<<<< HEAD:rdeprojector/vendor/k8s.io/api/rbac/v1beta1/doc.go
Copyright 2017 The Kubernetes Authors.
=======
Copyright 2021 The Kubernetes Authors.
>>>>>>> main:vendor/k8s.io/klog/v2/imports.go

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

<<<<<<< HEAD:rdeprojector/vendor/k8s.io/api/rbac/v1beta1/doc.go
// +k8s:deepcopy-gen=package
// +k8s:protobuf-gen=package
// +k8s:openapi-gen=true
// +k8s:prerelease-lifecycle-gen=true

// +groupName=rbac.authorization.k8s.io

package v1beta1 // import "k8s.io/api/rbac/v1beta1"
=======
package klog

import (
	"github.com/go-logr/logr"
)

// The reason for providing these aliases is to allow code to work with logr
// without directly importing it.

// Logger in this package is exactly the same as logr.Logger.
type Logger = logr.Logger

// LogSink in this package is exactly the same as logr.LogSink.
type LogSink = logr.LogSink

// Runtimeinfo in this package is exactly the same as logr.RuntimeInfo.
type RuntimeInfo = logr.RuntimeInfo

var (
	// New is an alias for logr.New.
	New = logr.New
)
>>>>>>> main:vendor/k8s.io/klog/v2/imports.go
