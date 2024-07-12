package deploy

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"gitee.com/dark.H/gn"
	"gitee.com/dark.H/gs"
)

func VulDel(url, api string) string {
	nt := gs.Str(url).AsRequest()
	nt = nt.SetMethod(gs.Str("DELETE")).SetHead("Authorization", gs.Str("Bearer %s").F(api))
	// nt.Color("g").Println(tag)
	r := gn.AsReq(nt).HTTPS()
	r.Build()

	if res := r.Go(); res.Str != "" {
		return res.Body().Str()
	}
	return ""
}

func VulGet(url string, api string) string {
	nt := gs.Str(url).AsRequest()
	nt = nt.SetMethod(gs.Str("GET")).SetHead("Authorization", gs.Str("Bearer %s").F(api))
	// nt.Color("g").Println(tag)
	r := gn.AsReq(nt).HTTPS()
	r.Build()

	if res := r.Go(); res.Str != "" {
		return res.Body().Str()
	}
	return ""
}

func VulPost(url, token, plat_id, region_id, my_tag string) string {
	nt := gs.Str(url).AsRequest()
	nt = nt.SetMethod(gs.Str("POST")).SetHead("Authorization", gs.Str("Bearer %s").F(token))
	nt = nt.SetHead("Content-Type", gs.Str("application/json"))
	nt = nt.SetBody(gs.Dict[any]{
		"region": region_id,
		"plan":   plat_id,
		"label":  my_tag,
		"os_id":  1743,
		"tags":   gs.List[string]{my_tag},
	}.Json())
	// nt.Color("g").Println(tag)
	r := gn.AsReq(nt).HTTPS()
	r.Build()

	if res := r.Go(); res.Str != "" {
		return res.Body().Str()
	}
	return ""
}

func Web_Vman(wt http.ResponseWriter, rq *http.Request) {
	// if !AuthCheck(wt, rq) {
	// 	return
	// }
	if rq.Method == "POST" {
		// parse json
		buf, err := ioutil.ReadAll(rq.Body)
		if err != nil {
			wt.WriteHeader(503)
			wt.Write([]byte("error:" + err.Error()))
			return
		}
		data := make(map[string]any)
		err = json.Unmarshal(buf, &data)
		if err != nil {
			wt.WriteHeader(503)
			wt.Write([]byte("error:" + err.Error()))
			return
		}
		if oper, ok := data["oper"]; ok {
			if Token, ok := data["token"]; ok {
				token := Token.(string)
				switch oper.(string) {
				case "del":
					fmt.Println(data)
					if ids, ok := data["ids"]; ok {
						if idss, ok := ids.([]any); ok {
							for _, id := range idss {
								if id, ok := id.(string); ok {
									gs.Str("del:%s").F(id).Color("r").Println()
									go func() {
										res := VulDel(gs.Str("https://api.vultr.com/v2/instances/%s").F(id).Str(), token)
										fmt.Println("Del:", res)
									}()
								}
							}
						}
					}
					wt.Write(gs.Str(`{"status":"ok"}`).Bytes())
				case "list":
					wt.Write([]byte(VulGet("https://api.vultr.com/v2/instances?per_page=100", token)))
				case "plans":
					wt.Write([]byte(VulGet("https://api.vultr.com/v2/plans?per_page=40", token)))
				case "regions":
					wt.Write([]byte(VulGet("https://api.vultr.com/v2/regions?per_page=40", token)))
				case "add":
					I := 0
					if plan_id, ok := data["plan_id"]; ok {
						if region_id, ok := data["region_id"]; ok {
							if CCC, ok := data["create_num"]; ok {
								create_num, _ := strconv.Atoi(CCC.(string))
								if regions := region_id.([]any); len(regions) > 0 {
									all_res_l := len(regions)
									for i := 0; i < create_num; i++ {
										region := regions[i%all_res_l].(string)
										if my_tag, ok := data["my_tag"]; ok {
											gs.Str("region:%s,plan:%s,tag:%s").F(region, plan_id.(string), my_tag.(string)).Color("g").Println("+")
											go func() {
												res := VulPost("https://api.vultr.com/v2/instances", token, plan_id.(string), region, my_tag.(string))
												gs.Str(res).Println("create")
											}()

											I += 1
										}
									}
								}

							}
						}
					}
					wt.Write([]byte(gs.Str(`{"status":"ok","num":%d}`).F(I)))
				}
			}
		}

		return
	}

	base := DefaultData()
	mapContent, _ := Render("/index/vman.html", nil)
	base.LayoutContent = mapContent
	// base2 := AddPage{}
	// base2.Type = "socks5"
	// add, _ := Render("/index/add.html", ObjToJsonMap(base2))
	// base.LayoutContent = add
	content, err := Render("/public/layout.html", base)
	if err != nil {
		wt.WriteHeader(400)
		wt.Write([]byte(content))
	} else {
		wt.Write([]byte(content))
	}
}
