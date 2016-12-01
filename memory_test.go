package cache

import (
	"fmt"
	"testing"
	"time"
)

func TestStd(t *testing.T) {
	cacher, err := NewCacher("memory", `{"interval":5,"expired":30}`)
	if err != nil {

	}

	for i := 0; i < 5000; i++ {
		cacher.Put(fmt.Sprintf("%v", i), "Name", int64(i))
	}

	if cacher.Len() != 5000 {
		t.Logf("Put was error %v/5000", cacher.Len())
	}

	ele := cacher.Get("1")
	fmt.Printf("5000=%v", ele)
	fmt.Println()
	fmt.Printf("active=%v  max=%v", cacher.Active(), cacher.Max())
	fmt.Println()
	fmt.Printf("Pop %v,%v/5000 ", cacher.Pop(), cacher.Len())
	fmt.Println()
	fmt.Printf("Shift %v,%v/5000 ", cacher.Shift(), cacher.Len())
	fmt.Println()

	fmt.Println(cacher.Active())
	for cacher.Len() != 0 {
		<-time.After(1 * time.Second)
		fmt.Println("Len", cacher.Len())
	}
}
