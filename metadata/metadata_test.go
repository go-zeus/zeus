package metadata

import (
	"context"
	"reflect"
	"testing"
)

func TestMDSet(t *testing.T) {
	ctx := Set(context.TODO(), "Key", "val")

	val, ok := Get(ctx, "Key")
	if !ok {
		t.Fatal("key Key not found")
	}
	if val != "val" {
		t.Errorf("key Key with value val != %v", val)
	}
}

func TestMDDelete(t *testing.T) {
	md := MD{
		"Foo": "bar",
		"Baz": "empty",
	}

	ctx := NewContext(context.TODO(), md)
	ctx = Delete(ctx, "Baz")

	emd, ok := FromContext(ctx)
	if !ok {
		t.Fatal("key Key not found")
	}

	_, ok = emd["Baz"]
	if ok {
		t.Fatal("key Baz not deleted")
	}

}

func TestMDCopy(t *testing.T) {
	md := MD{
		"Foo": "bar",
		"bar": "baz",
	}

	cp := Copy(md)

	for k, v := range md {
		if cv := cp[k]; cv != v {
			t.Fatalf("Got %s:%s for %s:%s", k, cv, k, v)
		}
	}
}

func TestMDContext(t *testing.T) {
	md := MD{
		"Foo": "bar",
	}

	ctx := NewContext(context.TODO(), md)

	emd, ok := FromContext(ctx)
	if !ok {
		t.Errorf("Unexpected error retrieving MD, got %t", ok)
	}

	if emd["Foo"] != md["Foo"] {
		t.Errorf("Expected key: %s val: %s, got key: %s val: %s", "Foo", md["Foo"], "Foo", emd["Foo"])
	}

	if i := len(emd); i != 1 {
		t.Errorf("Expected MD length 1 got %d", i)
	}
}

func TestMergeContext(t *testing.T) {
	type args struct {
		existing  MD
		append    MD
		overwrite bool
	}
	tests := []struct {
		name string
		args args
		want MD
	}{
		{
			name: "matching key, overwrite false",
			args: args{
				existing:  MD{"Foo": "bar", "Sumo": "demo"},
				append:    MD{"Sumo": "demo2"},
				overwrite: false,
			},
			want: MD{"Foo": "bar", "Sumo": "demo"},
		},
		{
			name: "matching key, overwrite true",
			args: args{
				existing:  MD{"Foo": "bar", "Sumo": "demo"},
				append:    MD{"Sumo": "demo2"},
				overwrite: true,
			},
			want: MD{"Foo": "bar", "Sumo": "demo2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := FromContext(MergeContext(NewContext(context.TODO(), tt.args.existing), tt.args.append, tt.args.overwrite)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MergeContext() = %v, want %v", got, tt.want)
			}
		})
	}
}
