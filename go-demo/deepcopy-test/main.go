package main

import "fmt"

type ObjectMeta struct {
	Name   string
	Labels map[string]string
}

func main() {
	in := &ObjectMeta{
		Name:   "vita",
		Labels: map[string]string{"label01": "test"},
	}

	out := &ObjectMeta{}

	in.DeepCopyInto(out)

}

func (in *ObjectMeta) DeepCopyInto(out *ObjectMeta) {
	*out = *in // out: {Name:vita,Labels:map[label01:test]}

	fmt.Printf("%p\n%p\n", out, in)               // 0x1b6fd735a030, 0x1b6fd735a018
	fmt.Printf("%p\n%p\n", out.Labels, in.Labels) // 0x1b6fd73ca0f0, 0x1b6fd73ca0f0

	if in.Labels != nil {
		out.Labels = make(map[string]string, len(in.Labels))
		for key, val := range in.Labels {
			out.Labels[key] = val
		}
	}
	fmt.Printf("%p\n%p\n", out.Labels, in.Labels) // 0x1b6fd73ca120, 0x1b6fd73ca0f0
	fmt.Println(in.Labels, out.Labels)            //  map[label01:test] map[label01:test]
}
