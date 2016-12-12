package cache

import (
	"fmt"
	"testing"
	"time"
	//	"time"
)

func TestStd(t *testing.T) {
	cacher, err := NewCacher("memory", `{"interval":5,"expired":2000}`)
	if err != nil {
		fmt.Println(err.Error())
	}

	for i := 0; i < 1000; i++ {
		go func(idx int, ck ICacher) {
			for k := 0; k < 1000; k++ {
				str := "Name"
				//ck.Put(fmt.Sprintf("%v%v", idx, k), str)
				ck.Push(str, int64(k))
			}

		}(i, cacher)
	}
	/*
		for i := 0; i < 100; i++ {
			go func(idx int, ck ICacher) {
				for {
					<-time.After(1 * time.Second)
					for k := 0; k < 10000; k++ {
						str := ""
						var ok bool
						itf := ck.Pop()
						if itf != nil {
							if str, ok = itf.(string); str == "" || !ok {
								//fmt.Println("Hello World")
								str = "Name"
							}
						}

						//ck.Put(fmt.Sprintf("%v%v", idx, k), str)
						ck.Push(str, int64(k))
					}
				}

			}(i, cacher)
		}
	*/
	//# 等待GO 程初始化
	<-time.After(2 * time.Second)

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
