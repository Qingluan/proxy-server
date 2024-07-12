package deploy

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"gitee.com/dark.H/gs"
	"github.com/c-bata/go-prompt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/manifoldco/promptui"
	"golang.org/x/term"
)

type GBook struct {
	Repo     gs.Str
	GPasswd  gs.Str
	User     gs.Str
	Pwd      gs.Str
	Logined  bool
	UsedBook gs.List[*Onevps]
}

func VpsToJson(i gs.List[*Onevps]) (text gs.Str, err error) {
	textBytes, err := json.Marshal(i)
	if err != nil {
		gs.Str(err.Error()).Color("r").Println("Err convert to vpss struct to json")
		return
	}
	text = gs.Str(textBytes)
	return
}

func JsonToVps(jsonBuf gs.Str) (ls gs.List[*Onevps], err error) {

	err = json.Unmarshal([]byte(jsonBuf), &ls)
	return
}

func SyncToGit(gitrepo, gitName, gitPwd, loginName, loginPwd string, vpss gs.List[*Onevps]) {
	// text := gs.Str("")
	// var err error
	// vpss.Every(func(no int, i *Onevps) {
	// 	text += gs.Str(i.Location + "|" + i.Host + "\n")
	// })

	text, err := VpsToJson(vpss)
	if err != nil {
		return
	}

	EncryptedText := text.Trim().Enrypt(loginPwd)
	tmprepo := gs.TMP.PathJoin("repot-sync-tmp")
	repoUrl := gitrepo
	if tmprepo.IsExists() {
		tmprepo.Rm()
	}
	repo, err := git.PlainClone(tmprepo.Str(), false, &git.CloneOptions{
		Auth: &githttp.BasicAuth{
			Username: gitName,
			Password: gitPwd,
		},
		URL:      repoUrl,
		Progress: os.Stdout,
	})
	if err != nil {
		gs.Str(err.Error()).Println("Err Clone")
		log.Fatal(err)
	}

	fileP := tmprepo.PathJoin(loginName)
	EncryptedText.ToFile(fileP.Str(), gs.O_NEW_WRITE)

	work, err := repo.Worktree()
	if err != nil {
		gs.Str(err.Error()).Println("Err")
		log.Fatal(err)
	}
	fileP.Color("b").Println("git:add file")
	_, err = work.Add(fileP.Basename().Str())
	if err != nil {
		gs.Str(err.Error()).Println("Err")
		log.Fatal(err)
	}
	_, err = work.Commit("example go-git commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "John Doe",
			Email: "john@doe.org",
			When:  time.Now(),
		},
	})

	if err != nil {
		gs.Str(err.Error()).Println("Err")
		log.Fatal(err)
	}
	gs.Str("Commit ok ").Color("g").Println("git")
	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
		// RefSpecs:   []config.RefSpec{config.RefSpec("+refs/heads/" + nameInfoObj.Version + ":refs/heads/" + nameInfoObj.Version)},
		Auth: &githttp.BasicAuth{
			Username: gitName,
			Password: gitPwd,
		},
	})
	if err != nil {
		gs.Str(err.Error()).Println("Err")
		log.Fatal(err)
	}
	gs.Str("Push ok ").Color("g").Println("git")
}

func (g *GBook) GitName() gs.Str {
	gitname := gs.Str(g.Repo).Split("/").Nth(3)
	return gitname
}

func (g *GBook) Login() {
	tmprepo := gs.TMP.PathJoin("repot-sync-tmp")
	if tmprepo.IsExists() {
		err := tmprepo.Rm()
		if err != nil {
			gs.Str(err.Error()).Println("Err")
			return
		}
	}
	gitname := g.GitName()
	if g.GPasswd != "" {
		_, err := git.PlainClone(tmprepo.Str(), false, &git.CloneOptions{
			Auth: &githttp.BasicAuth{
				Username: gitname.Str(),
				Password: g.GPasswd.Str(),
			},
			URL: g.Repo.Str(),
			// Progress: os.Stdout,
		})
		if err != nil {
			gs.Str(err.Error()).Add(gs.Str(tmprepo).Color("r")).Println("Err")
			return
		}

	} else {
		_, err := git.PlainClone(tmprepo.Str(), false, &git.CloneOptions{
			Auth: &githttp.BasicAuth{
				Username: gitname.Str(),
			},
			URL: g.Repo.Str(),
			// Progress: os.Stdout,
		})
		if err != nil {
			gs.Str(err.Error()).Add(gs.Str(tmprepo).Color("r")).Println("Err")
			return
		}
	}
	g.Logined = true

}

func (g *GBook) ChooseUser(user, pwd gs.Str) (err error) {
	if !g.Logined {
		g.Login()
	}
	g.User = user
	g.Pwd = pwd
	tmprepo := gs.TMP.PathJoin("repot-sync-tmp")
	filename := tmprepo.PathJoin(user.Str())
	name := user.Str()

	if !filename.IsExists() {
		gs.Str("Login Failed no such config ! "+name).Color("r", "B").Println("login")
		gs.Str(filename).Color("r", "B").Println("login")
		return
	} else {
		gs.Str("Login config ready!"+name).Color("g", "B").Println("login")
	}

	if encrpytedBuf := filename.MustAsFile(); encrpytedBuf != "" {
		if plain := encrpytedBuf.Derypt(pwd.Str()); plain.In(".") {
			g.UsedBook = gs.List[*Onevps]{}
			g.UsedBook, err = JsonToVps(plain)

		}
	} else {
		gs.Str("no buff !").Color("y").Println()
	}
	return
}

func (g *GBook) Update() {
	if g.UsedBook.Count() < 1 {
		gs.Str("no book to update!").Println()
		return
	}
	text, err := VpsToJson(g.UsedBook)
	if err != nil {
		return
	}

	EncryptedText := text.Trim().Enrypt(g.Pwd.Str())
	tmprepo := gs.TMP.PathJoin("repot-sync-tmp")
	if !tmprepo.IsExists() {
		gs.Str("repo not exists !!! can not update !!!!").Color("r").Println()
		return
	}

	repo, err := git.PlainOpen(tmprepo.Str())
	if err != nil {
		gs.Str(err.Error()).Println()
		return
	}

	fileP := tmprepo.PathJoin(g.User.Str())
	EncryptedText.ToFile(fileP.Str(), gs.O_NEW_WRITE)
	work, err := repo.Worktree()
	if err != nil {
		gs.Str(err.Error()).Println("Err")
		log.Fatal(err)
	}
	fileP.Color("b").Println("git: new book " + g.User)
	_, err = work.Add(fileP.Basename().Str())
	if err != nil {
		gs.Str(err.Error()).Println("Err")
		log.Fatal(err)
	}
	_, err = work.Commit(gs.Str("%s book changed: %s").F(g.User, time.Now()).Str(), &git.CommitOptions{
		Author: &object.Signature{
			Name:  string(gs.Str("").RandStr(5)),
			Email: string(gs.Str("").RandStr(5)) + "@mail.com",
			When:  time.Now(),
		},
	})

	if err != nil {
		gs.Str(err.Error()).Println("Err")
		log.Fatal(err)
	}
	gs.Str("Commit ok ").Color("g").Println("git")
	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
		// RefSpecs:   []config.RefSpec{config.RefSpec("+refs/heads/" + nameInfoObj.Version + ":refs/heads/" + nameInfoObj.Version)},
		Auth: &githttp.BasicAuth{
			Username: g.GitName().Str(),
			Password: g.GPasswd.Str(),
		},
	})
	if err != nil {
		gs.Str(err.Error()).Println("Err")
		log.Fatal(err)
	}
	gs.Str("Push ok ").Color("g").Println("git")
}

func (g *GBook) Clear() {
	tmprepo := gs.TMP.PathJoin("repot-sync-tmp")
	defer tmprepo.Rm()
}

func GitGetAccount(gitrepo string, namePwd ...string) (vpss gs.List[*Onevps]) {
	name := ""
	pwd := ""
	gitpwd := ""
	if namePwd != nil {
		name = namePwd[0]
		if len(namePwd) > 2 {
			pwd = namePwd[1]
			gitpwd = namePwd[2]
		} else if len(namePwd) > 1 {
			pwd = namePwd[1]

		}
	}
	tmprepo := gs.TMP.PathJoin("repot-sync-tmp")
	defer tmprepo.Rm()
	repoUrl := gitrepo
	if tmprepo.IsExists() {
		err := tmprepo.Rm()
		if err != nil {
			gs.Str(err.Error()).Println("Err")
			return
		}
	}
	if gitpwd != "" {
		gitname := gs.Str(gitrepo).Split("/").Nth(3)
		_, err := git.PlainClone(tmprepo.Str(), false, &git.CloneOptions{
			Auth: &githttp.BasicAuth{
				Username: gitname.Str(),
				Password: gitpwd,
			},
			URL: repoUrl,
			// Progress: os.Stdout,
		})
		if err != nil {
			gs.Str(err.Error()).Add(gs.Str(tmprepo).Color("r")).Println("Err")
			return
		}

	} else {
		_, err := git.PlainClone(tmprepo.Str(), false, &git.CloneOptions{
			URL: repoUrl,
			// Progress: os.Stdout,
		})
		if err != nil {
			gs.Str(err.Error()).Add(gs.Str(tmprepo).Color("r")).Println("Err")
			return
		}

	}
	reader := bufio.NewReader(os.Stdin)
	if name == "" {
		gs.Str("login name:").Color("u").Print()
		name, _ = reader.ReadString('\n')
		name = gs.Str(name).Trim().Str()
	}
	filename := tmprepo.PathJoin(name)
	if !filename.IsExists() {
		gs.Str("Login Failed no such config ! "+name).Color("r", "B").Println("login")
		gs.Str(filename).Color("r", "B").Println("login")
		return
	} else {
		gs.Str("Login config ready!"+name).Color("g", "B").Println("login")
	}

	if pwd == "" {
		gs.Str("login pwd:").Color("u").Print()
		pwd, _ = reader.ReadString('\n')
		pwd = gs.Str(pwd).Trim().Str()
	}
	if encrpytedBuf := filename.MustAsFile(); encrpytedBuf != "" {
		if plain := encrpytedBuf.Derypt(pwd); plain.In(".") {
			// plain.Println("pwd:" + pwd)
			if plain.StartsWith("[") && plain.EndsWith("]") {
				json.Unmarshal([]byte(plain), &vpss)
			} else {
				plain.EveryLine(func(lineno int, line gs.Str) {
					// line.Color("g").Println("Route")
					if line.In("|") {
						vpss = append(vpss, &Onevps{
							Location: line.Split("|").Nth(0).Trim().Str(),
							Host:     line.Split("|").Nth(1).Trim().Str(),
						})

					} else {
						vpss = append(vpss, &Onevps{
							Host: line.Trim().Str(),
						})
					}
				})

			}
			// var err error
			// vpss, err = JsonToVps(plain.Trim())
			// if
			gs.Str("login success !").Color("g", "B").Println("login")

		}
	} else {
		gs.Str("login password err !").Color("g", "B").Println("login")
	}

	return

}
func Getgitpass() gs.Str {
	// return gs.Str(prompt.Input("git passwd >", func(d prompt.Document) []prompt.Suggest { return []prompt.Suggest{} }, prompt.OptionInputBGColor(prompt.Color(prompt.))))
	gs.Str("Password > ").Print()
	bpasswd, _ := term.ReadPassword(int(os.Stdin.Fd()))
	return gs.Str(bpasswd)
}

func (g *GBook) Manager() {
	if !g.Logined {
		gs.Str("Can not manager !!!!").Println()
		return
	}

	for {

		ChangeFunc := func(d prompt.Document) []prompt.Suggest {
			s := []prompt.Suggest{
				{Text: "Password=", Description: "Store the vps's password."},
				{Text: "Location=", Description: "Store the vps's location."},
				{Text: "Label=", Description: "Store the vps' label "},
				{Text: "ok", Description: "Store the vps' label "},
				{Text: "save", Description: "save vps to remote "},
				{Text: "update", Description: "save vps to remote "},
				{Text: "push", Description: "save vps to remote "},
				{Text: "exit", Description: "Store the vps' label "},
			}
			return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), true)
		}

		tmprepo := gs.TMP.PathJoin("repot-sync-tmp")
		fs := gs.List[gs.Str]{}
		tmprepo.Ls().Filter(func(i gs.Str) bool {
			return !i.In("README") && !i.In(".git")
		}).Every(func(no int, i gs.Str) {
			fs = append(fs, i.Basename())
		})

		fs = append(fs, " ---  + New book ----")
		po := promptui.Select{
			Label: "What To Do ? Si",
			Items: fs,
			Searcher: func(input string, index int) bool {
				return gs.Str(fs[index]).In(input)
				// return false
			},
			StartInSearchMode: true,
			Size:              10,
		}
		noFs, _, err := po.Run()
		if err != nil {
			return
		}
		fname := fs[noFs]
		if fname == " ---  + New book ----" {
			for {
				fnameS := prompt.Input(fname.Trim().Str()+" New Book Name >", func(d prompt.Document) (_i []prompt.Suggest) { return }, prompt.OptionInputBGColor(prompt.Color(prompt.DisplayBold)))
				fname = gs.Str(fnameS).Trim()
				if fname.In(" ") {
					continue
				}
				if regexp.MustCompile(`^\w+$`).Match(fname.Bytes()) {
					break
				}
			}
		}

		pwd := Getgitpass().Trim().Str()
		// pwd := GetEdit(fname.Str() + " Password")
		g.ChooseUser(fname.Trim(), gs.Str(pwd).Trim())

		for {
			items := []string{"<< Add >>", "<< Update >>", "<< Build All Proxy >>", "<< Test Speed >>", "<<Exit>>"}
			paddingOpt := len(items)
			g.UsedBook.Every(func(no int, i *Onevps) {
				items = append(items, gs.Str(i.Host+" Loc: "+i.Location+" Label:"+i.Tag+" Password: "+i.Pwd).Color("g", "B").Str())
			})
			po := promptui.Select{
				Label: "What To Do ? Si",
				Items: items,
				Searcher: func(input string, index int) bool {
					return gs.Str(items[index]).In(input)
					// return false
				},
				StartInSearchMode: true,
				Size:              15,
			}
			no, _, err := po.Run()
			if err != nil {
				break
			}
			switch no {
			case 0:
				ip := prompt.Input("Input IP: ", func(d prompt.Document) (c []prompt.Suggest) {
					return
				})
				if gs.Str(ip).Count(".") == 3 {

					passwd := prompt.Input("Password: ", func(d prompt.Document) (c []prompt.Suggest) {
						return
					})
					label := prompt.Input("Label: ", func(d prompt.Document) (c []prompt.Suggest) {
						return
					})
					location := prompt.Input("Location: ", func(d prompt.Document) (c []prompt.Suggest) {
						return
					})
					vps := &Onevps{
						Host:     gs.Str(ip).Trim().Str(),
						Pwd:      strings.TrimSpace(passwd),
						Tag:      strings.TrimSpace(label),
						Location: location,
					}
					g.UsedBook = append(g.UsedBook, vps)
				}

			case 1:
				g.Update()
			case 2:
				waiter := sync.WaitGroup{}
				for _, v := range g.UsedBook {
					waiter.Add(1)
					go func(i *Onevps, q *sync.WaitGroup) {
						defer q.Done()
						i.Build()
					}(v, &waiter)
				}
				time.Sleep(300 * time.Millisecond)
				waiter.Wait()
			case 3:
				waiter := sync.WaitGroup{}
				for _, v := range g.UsedBook {
					waiter.Add(1)
					go func(i *Onevps, q *sync.WaitGroup) {
						defer q.Done()
						i.Test()
					}(v, &waiter)
				}
				time.Sleep(300 * time.Millisecond)
				waiter.Wait()
				g.UsedBook.Every(func(no int, i *Onevps) {
					gs.Str(i.Host + " Speed:" + i.Speed).Println()
				})
			case 4:
				break
			default:
				vps := g.UsedBook[no-paddingOpt]
				po := promptui.Select{
					Label: vps.Host + " >",
					Items: []string{"Edit", "Build Proxy", "SSH", "Delete"},
				}

				no, _, err := po.Run()
				if err != nil {
					break
				}
				switch no {
				case 1:

					vps.Build()
				case 2:
					SSHCli("root@" + vps.Host + ":22" + "/" + vps.Pwd)
				case 0:
					ifupdate := false
					for {
						t := strings.TrimSpace(prompt.Input(g.User.Str()+"ip: "+vps.Host+" >", ChangeFunc))
						if t == "" || t == "ok" || t == "exit" {
							break
						}
						if t == "save" || t == "update" || t == "commit" || t == "push" {
							ifupdate = true
							break
						}
						ts := gs.Str(t)
						if ts.In("=") {
							es := ts.Split("=", 2)
							switch es[0].Trim() {
							case "Password":
								vps.Pwd = es[1].Trim().Str()
								gs.Str(es[0].Trim()+" -> "+es[1].Trim()).Color("g", "B").Println()
							case "Location":
								vps.Location = es[1].Trim().Str()
								gs.Str(es[0].Trim()+" -> "+es[1].Trim()).Color("g", "B").Println()
							case "Label":
								vps.Tag = es[1].Trim().Str()
								gs.Str(es[0].Trim()+" -> "+es[1].Trim()).Color("g", "B").Println()
							case "Tag":
								vps.Tag = es[1].Trim().Str()
								gs.Str(es[0].Trim()+" -> "+es[1].Trim()).Color("g", "B").Println()
							}
						} else {
							o, _ := json.MarshalIndent(vps, "\n", "    ")
							gs.Str(o).Color("y").Println()
						}

					}
					if ifupdate {
						g.Update()
					}

				}

			}

		}
	}

}

func (gbook *GBook) Upload(vpses gs.List[*Onevps]) bool {
	user := ""

	for _, vps := range vpses {
		user = strings.TrimSpace(vps.Tag)
	}
	if user != "" {
		pwd := GetEdit("Set <" + user + "> login password")
		if pwd.Trim() != "" {
			gbook.Pwd = pwd
			gbook.User = gs.Str(user)
			gbook.UsedBook = vpses
			gbook.Update()
			return true
		}
	}
	return false
}
