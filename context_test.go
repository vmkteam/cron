package cron

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type (
	keyString  struct{}
	keyInt     struct{}
	keyBool    struct{}
	keyPtr     struct{}
	keyStructA struct{}
	keyStructB struct{}
)

type sampleStruct struct {
	ID   int
	Name string
}

func TestCtxValue(t *testing.T) {
	Convey("ContextValue should correctly store and retrieve values", t, func() {
		Convey("string value", func() {
			cv := NewContextValue[keyString, string]()
			ctx := cv.WithValue(context.Background(), "hello")
			So(cv.FromContext(ctx), ShouldEqual, "hello")
		})

		Convey("int value", func() {
			cv := NewContextValue[keyInt, int]()
			ctx := cv.WithValue(context.Background(), 123)
			So(cv.FromContext(ctx), ShouldEqual, 123)
		})

		Convey("bool value", func() {
			cv := NewContextValue[keyBool, bool]()
			ctx := cv.WithValue(context.Background(), true)
			So(cv.FromContext(ctx), ShouldBeTrue)
		})

		Convey("pointer value", func() {
			cv := NewContextValue[keyPtr, *int]()
			val := 42
			ctx := cv.WithValue(context.Background(), &val)
			got := cv.FromContext(ctx)
			So(got, ShouldNotBeNil)
			So(*got, ShouldEqual, 42)
		})

		Convey("struct value", func() {
			cv := NewContextValue[keyStructA, sampleStruct]()
			value := sampleStruct{ID: 1, Name: "test"}
			ctx := cv.WithValue(context.Background(), value)
			So(cv.FromContext(ctx), ShouldResemble, value)
		})

		Convey("absent value returns zero", func() {
			cv := NewContextValue[keyString, string]()
			So(cv.FromContext(context.Background()), ShouldEqual, "")
		})

		Convey("different keys do not conflict", func() {
			cvA := NewContextValue[keyStructA, string]()
			cvB := NewContextValue[keyStructB, string]()
			ctx := cvA.WithValue(context.Background(), "valueA")
			So(cvA.FromContext(ctx), ShouldEqual, "valueA")
			So(cvB.FromContext(ctx), ShouldEqual, "")
		})

		Convey("value can be overwritten", func() {
			cv := NewContextValue[keyString, string]()
			ctx := context.Background()
			ctx = cv.WithValue(ctx, "first")
			ctx = cv.WithValue(ctx, "second")
			So(cv.FromContext(ctx), ShouldEqual, "second")
		})
	})
}
