// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package editionssupport defines constants for editions that are supported.
package editionssupport

<<<<<<< HEAD
import "google.golang.org/protobuf/types/descriptorpb"
=======
import descriptorpb "google.golang.org/protobuf/types/descriptorpb"
>>>>>>> dc30ffe8 (feat: add port filter option alongside Multitenancy and bug fixes)

const (
	Minimum = descriptorpb.Edition_EDITION_PROTO2
	Maximum = descriptorpb.Edition_EDITION_2023
<<<<<<< HEAD

	// MaximumKnown is the maximum edition that is known to Go Protobuf, but not
	// declared as supported. In other words: end users cannot use it, but
	// testprotos inside Go Protobuf can.
	MaximumKnown = descriptorpb.Edition_EDITION_2024
=======
>>>>>>> dc30ffe8 (feat: add port filter option alongside Multitenancy and bug fixes)
)
