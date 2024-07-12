package deploy

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"gitee.com/dark.H/gn"
	"gitee.com/dark.H/gs"
	"github.com/c-bata/go-prompt"
	"github.com/manifoldco/promptui"
)

type GV struct {
	API          gs.Str
	lock         sync.RWMutex
	createOption gs.Dict[any]
	createdVPS   gs.List[*Onevps]
}

func (gv *GV) base(uri string, method string, data ...gs.Dict[any]) (out gs.Dict[any]) {
	req := gs.Str("https://api.vultr.com/v2"+uri).AsRequest().SetHead("Authorization", gs.Str(fmt.Sprintf("Bearer %s", gv.API)))
	req = req.SetMethod(gs.Str(method))
	req = req.SetHead("Content-Type", "application/json")
	// if method == "POST" {

	// }
	if data != nil && data[0] != nil {
		req = req.SetBody(data[0].Json())

	}

	req.HTTPS = true
	// req.Println()
	R := gn.AsReq(req).HTTPS()
	// R.Println()
	if res := R.Go(); res.Err != nil {
		// res.Println("res")
		gs.Str(res.Err.Error()).Color("r").Println("base request")
		return
	} else {
		if method == "DELETE" {
			return
		}
		return res.Body().Json()
	}
}

func (gv *GV) CreateInstances() {
	gv.createOption = gs.Dict[any]{
		"region": "",
		"plan":   "",
		"os_id":  1743,
		"label":  "",
		"tags":   []string{},
	}
	gv.createdVPS = gs.List[*Onevps]{}
	canusedRegions := map[string]string{}
CreateLoop:
	for {
		kw := strings.TrimSpace(prompt.Input("Set Create Instance Options > ", func(d prompt.Document) []prompt.Suggest {
			s := []prompt.Suggest{
				{
					Text:        "Create",
					Description: "Try To Use Now Options to create vps!!",
				},
				{
					Text:        "Ok",
					Description: "Try To Use Now Options to create vps!!",
				},
				{
					Text:        "exit",
					Description: "exit",
				},
			}
			for k := range gv.createOption {
				s = append(s, prompt.Suggest{
					Text: k, Description: "Set " + k + "'s value",
				})
			}
			return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), true)
		}))
		// choosed := false
		switch kw {
		case "region":
			res := gv.base("/regions", "GET")
			// plans := res["regions"].([]any)
			if len(canusedRegions) > 0 {
				regions := []string{}
				plans := []any{}
				for _, p := range res["regions"].([]any) {
					id := p.(map[string]any)["id"].(string)

					if _, ok := canusedRegions[id]; ok {
						canusedRegions[p.(map[string]any)["id"].(string)] = p.(map[string]any)["city"].(string)
						plans = append(plans, p)
						// gs.Str(id).Println("id")
					}
				}

				index := 2
				var err error
				for index > 0 {
					prompt := promptui.Select{
						Label: "Used Regions",
						Items: append([]string{"---- Save ----", "---- Add ----"}, regions...),
						Size:  10,
					}

					index, _, err = prompt.Run()
					if err != nil {
						continue
					}
					if index == 0 {
						break
					} else if index == 1 {
						item, choosed := Select("Choose VPS region", plans, func(a any) string {
							ee := a.(map[string]any)
							return fmt.Sprintf("(%v)%v - %v ", ee["id"], ee["city"], ee["continent"])
						})
						if choosed {
							id := item.(map[string]any)["id"]
							regions = append(regions, id.(string))
							gv.createOption[kw] = strings.Join(regions, ",")
							gs.Str(kw+"-> "+gs.Str(strings.Join(regions, ",")).Str()).Color("g", "B").Println("+")

						}

					}
				}
			} else {
				gs.Str("Must set plan first !!!").Color("r").Println("!")
			}
		case "os_id":
			if len(canusedRegions) > 0 {
				res := gv.base("/os", "GET")
				plans := res["os"].([]any)

				item, choosed := Select("Choose VPS Operation System", plans, func(a any) string {
					ee := a.(map[string]any)
					return fmt.Sprintf("(%v)%v - %v ", ee["id"], ee["name"], ee["arch"])
				})
				if choosed {
					id := item.(map[string]any)["id"]
					gs.Str(kw+"-> "+gs.S(id).Str()).Color("g", "B").Println("+")
					gv.createOption[kw] = id
				}

			} else {
				gs.Str("Must set plan first !!!").Color("r").Println("!")
			}
		case "tags":
			gs.S(gv.createOption["tags"]).Println("now tags")
			tags := []string{}

			for {
				t := prompt.Input("tags>", func(d prompt.Document) (e []prompt.Suggest) {
					e = append(e, prompt.Suggest{
						Text:        "exit",
						Description: "exit add tags",
					})
					return
				})
				tt := gs.Str(t)
				if tt.Trim() == "exit" {
					break
				}
				if !tt.In(" ") && !tt.In("/") && !tt.In("\\") {
					tags = append(tags, tt.Trim().Str())
				}
			}
			gv.createOption["tags"] = tags
			if len(tags) > 0 {
				gv.createOption["label"] = tags[0]
			}

		case "plan":
			res := gv.base("/plans", "GET")
			plans := res["plans"].([]any)
			item, choosed := Select("Choose VPS Operation System", plans, func(a any) string {
				ee := a.(map[string]any)
				return fmt.Sprintf("(%v) ram:%vMB - %v$/mon type:%v locs:%v", ee["id"], ee["ram"], ee["monthly_cost"], ee["type"], ee["locations"])
			})

			if choosed {
				canusedRegions = make(map[string]string)
				id := item.(map[string]any)["id"]
				for _, k := range item.(map[string]any)["locations"].([]any) {
					canusedRegions[k.(string)] = ""
				}
				gs.Str(kw+"-> "+gs.S(id).Str()).Color("g", "B").Println("+")
				gv.createOption[kw] = id
			}
		case "Create", "Ok":
			if id, ok := gv.createOption["plan"]; ok && id != "" {
				if os_id, ok := gv.createOption["os_id"]; ok && os_id != "" {
					if label, ok := gv.createOption["label"]; ok && label != "" {
						if regions, ok := gv.createOption["region"]; ok && regions != "" {
							if strings.Contains(regions.(string), ",") {
								rs := strings.Split(regions.(string), ",")
								wait := sync.WaitGroup{}
								for _, loc := range rs {
									wait.Add(1)
									option := gs.Dict[any]{
										"plan":                gv.createOption["plan"],
										"os_id":               gv.createOption["os_id"],
										"tags":                gv.createOption["tags"],
										"label":               gv.createOption["label"],
										"disable_public_ipv4": false,
										"region":              loc,
									}
									go func(op gs.Dict[any], wa *sync.WaitGroup) {
										defer wa.Done()
										res := gv.base("/instances", "POST", option)
										if res != nil {
											vps_info := res["instance"].(map[string]any)
											ip := vps_info["main_ip"].(string)
											id := vps_info["id"].(string)
											gs.Str(ip + " wait regist IP").Println(id)
											for ip == "0.0.0.0" || ip == "" {
												time.Sleep(2 * time.Second)
												if res2 := gv.base("/instances/"+id, "GET"); res2 != nil && res2["instance"] != nil {
													ip = res2["instance"].(map[string]any)["main_ip"].(string)
												}
											}
											gv.lock.Lock()
											v := &Onevps{
												Host:     ip,
												Location: canusedRegions[vps_info["region"].(string)],
												Pwd:      vps_info["default_password"].(string),
												Tag:      vps_info["label"].(string),
											}
											gv.createdVPS = append(gv.createdVPS, v)
											v.Println()
											gv.lock.Unlock()

										}

									}(option, &wait)
								}
								time.Sleep(3 * time.Second)
								wait.Wait()

								break CreateLoop
							}
						}
					}
				}
			}
		case "exit":
			break CreateLoop

		}
		for k, v := range gv.createOption {
			gs.S(v).Println(k)
		}
		ss := ""
		for k := range canusedRegions {
			ss += k + ","
		}
		gs.Str(ss).Println("loc")
	}
	if len(gv.createdVPS) > 0 {
		saved := false
		for !saved {
			gitrepo := prompt.Input("git repo (default gitee)> ", func(d prompt.Document) (e []prompt.Suggest) { return })
			if !strings.HasPrefix(gitrepo, "https") {
				gitrepo = "https://gitee.com/" + gitrepo
			}
			gb := &GBook{
				Repo:    gs.Str(gitrepo),
				GPasswd: Getgitpass(),
			}
			gb.Login()
			if gb.Logined {
				if gb.Upload(gv.createdVPS) {
					saved = true
				}
				gb.Clear()
			}
		}
	}
}

func (gv *GV) List() {
	res := gv.base("/instances", "GET")
	if res != nil {
		instances := res["instances"].([]any)
	M:
		for {
			ins, ee := Select("Choose instance", instances, func(a any) string {
				aa := a.(map[string]any)
				return "label: " + gs.S(aa["label"]).Str() + " IP:" + gs.S(aa["main_ip"]).Str() + " " + gs.S(aa["region"]).Str() + " create:" + aa["date_created"].(string)
			})
			if ee {
				for {
					if a, cc := Select("Choose Do", []any{"del", "ok"}, func(a any) string {
						return a.(string)
					}); cc {
						if a == "del" {
							gv.base("/instances/"+ins.(map[string]any)["id"].(string), "DELETE")
							gs.Str("Try Del : " + ins.(map[string]any)["id"].(string)).Color("y").Println("!")
							break
						} else {
							break M
						}
					}

				}
			} else {
				break M
			}
		}

	}

}

func Select(prefix string, items []any, strfunc func(any) string) (item any, chooed bool) {
	fs := []string{}
	for _, a := range items {
		fs = append(fs, strfunc(a))
	}
	po := promptui.Select{
		Label: "Select",
		Items: fs,
		Searcher: func(input string, index int) bool {
			return gs.Str(fs[index]).In(input)
			// return false
		},
		StartInSearchMode: true,
		Size:              20,
	}
	no, _, err := po.Run()
	if err == nil {
		chooed = true
		return items[no], chooed
	}
	return nil, false
}
