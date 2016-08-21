//Package goScript, javascript Eval() for go
//The MIT License (MIT)
//Copyright (c) 2016 Juan Pascual

package goScript

import (
	"fmt"
	"go/ast"
	"reflect"
)

type callSite struct {
	callee  interface{}
	fnName  string
	isValid bool
}

func evalSelectorExpr(expr *ast.SelectorExpr, context Context) (interface{}, error) {
	callee, err := eval(expr.X, context)
	if err != nil {
		return nilInterf, err
	}

	calleeVal := reflect.ValueOf(callee)
	fieldVal := calleeVal
	if fieldVal.Kind() == reflect.Ptr {
		fieldVal = calleeVal.Elem() //FieldByName panics on pointers
	}
	sName := expr.Sel.Name
	fbnVal, ok := fieldVal.Type().FieldByName(sName) //Faster than  fieldVal.FieldByName(sName)
	if ok {
		return fieldVal.FieldByIndex(fbnVal.Index).Interface(), nil
	}

	mval := fieldVal.MethodByName(sName)
	if !mval.IsValid() {
		return nilInterf, fmt.Errorf("Selector %s not field nor method", sName)
	}

	//Return function pointer
	return mval.Pointer(), nil
}

func evalSelectorExprCall(expr *ast.SelectorExpr, context Context) (interface{}, error, callSite) {
	callee, err := eval(expr.X, context)
	if err != nil {
		return nilInterf, err, callSite{isValid: false}
	}
	return nil, nil, callSite{callee, expr.Sel.Name, true}
}

func evalCallExpr(expr *ast.CallExpr, context Context) (interface{}, error) {

	//Find the type called, this calls evalSelectorExpr/ evalIdent
	val, err, callsite := evalFromCall(expr.Fun, context)
	if err != nil {
		return nilInterf, err
	}

	//-------------------Check Method------------------------
	var vaArgsTp reflect.Type
	var method reflect.Value

	if !callsite.isValid {
		//Not a callSite, must be a Ident(args), so Ident must be a function
		method = reflect.ValueOf(val)
		if method.Kind() != reflect.Func {
			return nilInterf, fmt.Errorf("Waiting reflect.Func found %v", reflect.TypeOf(val))
		}
	} else {
		//A callSite so must be f.x(args)
		caleeVal := reflect.ValueOf(callsite.callee)
		method = caleeVal.MethodByName(callsite.fnName)
		if !method.IsValid() {
			return nilInterf, fmt.Errorf("Method %s not found", callsite.fnName)
		}
	}
	mType := method.Type()
	numArgs := mType.NumIn()

	if !mType.IsVariadic() {
		if len(expr.Args) != numArgs {
			return nilInterf, fmt.Errorf("Method alguments count mismatch. Expected %d get %d", numArgs, len(expr.Args))
		}
	} else {
		numArgs = numArgs - 1
		vaArgsTp = mType.In(numArgs).Elem() //Type declared
		if len(expr.Args) < numArgs {
			return nilInterf, fmt.Errorf("Method alguments count mismatch. Expected at least %d get %d", (numArgs - 1), len(expr.Args))
		}
	}

	//-------------------Prepare call arguments ------------
	var args []reflect.Value
	if len(expr.Args) == 0 {
		args = zeroArg //Zero arg constant
	} else {

		args = make([]reflect.Value, len(expr.Args))

		for key, value := range expr.Args {

			val, err := eval(value, context)
			if err != nil {
				return nilInterf, err
			}
			rVal := reflect.ValueOf(val)
			var tArg reflect.Type //Method argument type

			//If true we are in the variadic parameters
			if key >= numArgs {
				tArg = vaArgsTp
			} else {
				tArg = mType.In(key)
			}
			tVal := rVal.Type() //Passed parameter type
			if tVal != tArg {
				if !tVal.ConvertibleTo(tArg) {
					return nilInterf, fmt.Errorf("Method argument %d type mismatch. Expected %s get %s", key, tArg, tVal)
				}
				rVal = rVal.Convert(tArg)
			}
			args[key] = rVal
		}
	}
	//Call
	retVal := method.Call(args)

	//Evaluate result
	if len(retVal) == 0 {
		return nilInterf, nil
	}
	return retVal[0].Interface(), nil
}
