package service

import "github.com/kumbuka-me/kumbuka/pkg/domain"

// FieldError is shared with repositories so field information survives every layer.
type FieldError = domain.FieldError

// ValidationError is the common application and repository validation contract.
type ValidationError = domain.ValidationError
