/*
 Copyright 2021 The CloudEvents Authors
 SPDX-License-Identifier: Apache-2.0
*/

package expression

import (
	cesql "github.com/cloudevents/sdk-go/sql/v2"
	sqlerrors "github.com/cloudevents/sdk-go/sql/v2/errors"
	"github.com/cloudevents/sdk-go/sql/v2/utils"
	cloudevents "github.com/cloudevents/sdk-go/v2"
)

type mathExpression struct {
	baseBinaryExpression
	fn func(x, y int32) (int32, error)
}

func (s mathExpression) Evaluate(event cloudevents.Event) (interface{}, error) {
	leftVal, err := s.left.Evaluate(event)
	if err != nil {
		return int32(0), err
	}

	rightVal, err := s.right.Evaluate(event)
	if err != nil {
		return int32(0), err
	}

	leftVal, err = utils.Cast(leftVal, cesql.IntegerType)
	if err != nil {
		return int32(0), err
	}

	rightVal, err = utils.Cast(rightVal, cesql.IntegerType)
	if err != nil {
		return int32(0), err
	}

	return s.fn(leftVal.(int32), rightVal.(int32))
}

func NewSumExpression(left cesql.Expression, right cesql.Expression) cesql.Expression {
	return mathExpression{
		baseBinaryExpression: baseBinaryExpression{
			left:  left,
			right: right,
		},
		fn: func(x, y int32) (int32, error) {
			return x + y, nil
		},
	}
}

func NewDifferenceExpression(left cesql.Expression, right cesql.Expression) cesql.Expression {
	return mathExpression{
		baseBinaryExpression: baseBinaryExpression{
			left:  left,
			right: right,
		},
		fn: func(x, y int32) (int32, error) {
			return x - y, nil
		},
	}
}

func NewMultiplicationExpression(left cesql.Expression, right cesql.Expression) cesql.Expression {
	return mathExpression{
		baseBinaryExpression: baseBinaryExpression{
			left:  left,
			right: right,
		},
		fn: func(x, y int32) (int32, error) {
			return x * y, nil
		},
	}
}

func NewModuleExpression(left cesql.Expression, right cesql.Expression) cesql.Expression {
	return mathExpression{
		baseBinaryExpression: baseBinaryExpression{
			left:  left,
			right: right,
		},
		fn: func(x, y int32) (int32, error) {
			if y == 0 {
				return 0, sqlerrors.NewMathError("division by zero")
			}
			return x % y, nil
		},
	}
}

func NewDivisionExpression(left cesql.Expression, right cesql.Expression) cesql.Expression {
	return mathExpression{
		baseBinaryExpression: baseBinaryExpression{
			left:  left,
			right: right,
		},
		fn: func(x, y int32) (int32, error) {
			if y == 0 {
				return 0, sqlerrors.NewMathError("division by zero")
			}
			return x / y, nil
		},
	}
}
