package memory

import (
	"fmt"
	"sync"
	"testing"

	"github.com/volts-dev/cacher"
	"github.com/volts-dev/utils"
)

func TestConfig(t *testing.T) {
	cfg := Config{}
	cfg.Init(
		WithInterval(15),
	)
	fmt.Println(cfg)
}

func TestStd(t *testing.T) {
	chr := New()
	count := 5
	var w sync.WaitGroup
	w.Add(count)
	for i := 0; i < count; i++ {
		go func(idx int, ck cacher.ICacher) {
			for k := 0; k < 1000; k++ {
				str := utils.IntToStr(idx) + "Name" + utils.IntToStr(k)
				ck.Set(&cacher.CacheBlock{Key: utils.IntToStr(idx) + "+" + utils.IntToStr(k), Value: str})
			}
			w.Done()
		}(i, chr)
	}

	w.Wait() //# 等待GO程初始化
	if chr.Len() != count*1000 {
		t.Logf("Put was error %v/%d", chr.Len(), count*1000)
	}

	var ele string
	for i := 0; i < count; i++ {
		for k := 0; k < 1000; k++ {
			err := chr.Get(utils.IntToStr(i)+"+"+utils.IntToStr(k), &ele)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Println(fmt.Sprintf("%d=%v", i, ele))
		}
	}
	/*
		for i := 0; i < 100; i++ {
			go func(idx int, ck cacher.ICacher) {
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

			}(i, chr)
		}
	*/

	fmt.Println()
	//fmt.Printf("active=%v  max=%v", cacher.Active(), cacher.Max())
	fmt.Println()
	//fmt.Printf("Pop %v,%v/5000 ", cacher.Pop(), cacher.Len())
	fmt.Println()
	//	fmt.Printf("Shift %v,%v/5000 ", cacher.Shift(), cacher.Len())
	fmt.Println()
	fmt.Println(chr.Active())
}
