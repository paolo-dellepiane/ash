package main

// Host (.*)HostName (.*)User (.*)IdentityFile (.*)$
import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/c-bata/go-prompt"
	"github.com/jinzhu/configor"
)

var version, sha1ver, buildTime string

var cfg struct {
	Profiles                                    []string
	AwsInstances, KeysPath, SSHConfig, AppendTo string
}

func entry(s Server) string {
	user := "ubuntu"
	if s.Platform == "windows" {
		user = "administrator"
	}
	return fmt.Sprintf(`# generated [%s]
Host %s
    HostName %s
    User %s
    IdentityFile %s
`, s.Profile, strings.ReplaceAll(s.Name, " ", ""), s.Address, user, s.Key)
}

func extract(in [][]string, idx int) []string {
	var res []string
	for _, v := range in {
		res = append(res, v[idx])
	}
	return res
}

func distinct(intSlice []string) []string {
	keys := make(map[string]bool)
	out := []string{}
	for _, entry := range intSlice {
		if _, ok := keys[entry]; !ok {
			keys[entry] = true
			out = append(out, entry)
		}
	}
	return out
}

func lookForPath(filePath string) string {
	if fileExists(filePath) {
		return filePath
	}
	return inExeDir(filePath)
}

func inExeDir(filePath string) string {
	ex, _ := os.Executable()
	tmp := path.Join(filepath.Dir(ex), filePath)
	if !fileExists(tmp) {
		f, _ := os.Create(tmp)
		f.Close()
	}
	return tmp
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func update() {
	out := os.Stdout
	if !*printFlag {
		out, _ = os.Create(cfg.SSHConfig)
		defer out.Close()
	}
	if appendTo, err := os.ReadFile(cfg.AppendTo); err == nil {
		out.Write(appendTo)
	}
	for _, p := range cfg.Profiles {
		for _, v := range Instances(p, cfg.KeysPath) {
			fmt.Fprintf(out, "%s\n", entry(v))
		}
	}
}

func inputStrings(prefix string, values []string, in ...string) (int, string, error) {
	suggests := make([]prompt.Suggest, len(values))
	for i, v := range values {
		suggests[i] = prompt.Suggest{Text: v}
	}
	return inputSuggests(prefix, suggests, in...)
}

func inputSuggests(prefix string, suggests []prompt.Suggest, in ...string) (int, string, error) {
	completer := func(d prompt.Document) []prompt.Suggest {
		return prompt.FilterFuzzy(suggests, d.GetWordBeforeCursor(), true)
	}
	var res string
	if len(in) > 0 && in[0] != "" {
		res = in[0]
	} else {
		res = prompt.Input(prefix, completer, prompt.OptionShowCompletionAtStart(), prompt.OptionCompletionOnDown(), prompt.OptionSuggestionBGColor(prompt.DarkGray))
	}
	var selected []prompt.Suggest
	for _, e := range suggests {
		if e.Text == res {
			selected = append(selected, e)
		}
	}
	if len(selected) == 0 {
		selected = prompt.FilterFuzzy(suggests, res, true)
	}
	if len(selected) == 0 {
		return -1, "", errors.New("can't find " + res)
	}
	for i, s := range suggests {
		if selected[0] == s {
			return i, s.Text, nil
		}
	}
	return -1, "", errors.New("can't find " + res)
}

func getServer() *Server {
	if !fileExists(historyPath) {
		os.WriteFile(historyPath, []byte(`[]`), os.ModeAppend)
	}
	historyFile, _ := os.ReadFile(historyPath)
	var history []string
	json.Unmarshal(historyFile, &history)
	f, _ := os.ReadFile(cfg.SSHConfig)
	allProfiles := regexp.MustCompile(`(?smU)# generated \[(.*)\].*$`).FindAllStringSubmatch(string(f), -1)
	profiles := append([]string{`history`, `all`}, distinct(extract(allProfiles, 1))...)
	i, profile, err := inputStrings("> ", profiles, flag.Arg(0))
	entriesrxstr := `(?smU)# generated \[` + profile + `\].*\r?$.*Host (.*)\r?$.*HostName (.*)\r?$.*User (.*)\r?$.*IdentityFile (.*)\r?$`
	if i < 2 {
		entriesrxstr = `(?smU)Host (.*)\r?$.*HostName (.*)\r?$.*User (.*)\r?$.*IdentityFile (.*)\r?$`
	}
	entries := regexp.MustCompile(entriesrxstr).FindAllStringSubmatch(string(f), -1)
	val := flag.Arg(1)
	if profile == `history` {
		_, val, _ = inputStrings(profile+"> ", history)
	}
	i, _, err = inputStrings(profile+"> ", extract(entries, 1), val)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	srv := &Server{entries[i][1], entries[i][3] + "@" + entries[i][2], profile, entries[i][4], ""}
	history = distinct(append([]string{srv.Name}, history...))
	b, _ := json.Marshal(history)
	_ = os.WriteFile(historyPath, b, os.ModeAppend)
	return srv
}

func info() {
	fmt.Println("Version:", version)
	fmt.Println("Build:", sha1ver, buildTime)
	fmt.Println("History:", historyPath)
	fmt.Println("Configuration:", fmt.Sprintf("%+v", cfg))
}

func serverInfo() {
	s := getServer()
	if *infoFlag {
		out := os.Stdout
		if !*printFlag {
			out, _ = os.Create(*oFlag)
			defer out.Close()
		}
		out.WriteString(fmt.Sprintf(`"%s","%s","%s","%s"`, s.Profile, s.Name, s.Key, s.Address))
		return
	}
}

func ssh() {
	s := getServer()
	if *putFlag != "" {
		executeInteractive(`scp`, `-i`, s.Key, *putFlag, s.Address+`:`)
	} else if *getFlag != "" {
		executeInteractive(`scp`, `-i`, s.Key, s.Address+`:`+*getFlag, `.`)
	} else if *execFlag != "" {
		executeInteractive(`ssh`, `-i`, s.Key, s.Address, *execFlag)
	} else {
		executeInteractive(`ssh`, `-i`, s.Key, s.Address)
	}
}

func execute(name string, arg ...string) string {
	fmt.Println("execute", name, strings.Join(arg, " "))
	cmd := exec.Command(name, arg...)
	cmd.Stdin = strings.NewReader("mah")
	o, _ := cmd.CombinedOutput()
	return string(o)
}

func executeInteractive(name string, arg ...string) {
	fmt.Println("execute", name, strings.Join(arg, " "))
	cmd := exec.Command(name, arg...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func vsdbg() {
	s := getServer()
	ls := execute(`ssh`, `-i`, s.Key, s.Address, `sudo docker ps --format "{{.ID}},{{.Names}},{{.Image}}"`)
	os.WriteFile("tmp", []byte(ls), os.ModeAppend)
	containers := [][]string{}
	var cNames []prompt.Suggest
	for _, l := range strings.Split(ls, "\n") {
		x := strings.Split(l, ",")
		if len(x) > 1 {
			containers = append(containers, x)
			cNames = append(cNames, prompt.Suggest{Text: x[1], Description: x[2]})
		}
	}
	i, _, _ := inputSuggests("", cNames)
	executeInteractive(`scp`, `-i`, s.Key, inExeDir(`utils/vsdbg.sh`), s.Address+`:`)
	executeInteractive(`ssh`, `-i`, s.Key, s.Address, `sudo`, `bash`, `vsdbg.sh`, containers[i][0], *vsdbgPortFlag)
	fmt.Println("SSH to", s.Address, *vsdbgPortFlag)
}

var updateFlag = flag.Bool("update", false, "update ssh config file (path in config)")
var oFlag = flag.String("o", "out", "output file")
var printFlag = flag.Bool("print", false, "print to stdout")
var cfgFlag = flag.String("cfg", "ash.config.json", "ash config file")
var putFlag = flag.String("put", "", "put file or directory")
var getFlag = flag.String("get", "", "get file or directory")
var execFlag = flag.String("exec", "", "execute command")
var versionFlag = flag.Bool("version", false, `print version`)
var infoFlag = flag.Bool("info", false, `print selected server info to file specified by "o" flag`)
var serverFlag = flag.Bool("server", false, `print selected server info to file specified by "o" flag`)
var vsdbgFlag = flag.Bool("vsdbg", false, `setup .net remote container debug`)
var vsdbgPortFlag = flag.String("vsdbgport", "4444", `.net remote container port`)
var historyPath = `history`

func main() {
	flag.Parse()
	*cfgFlag = lookForPath(*cfgFlag)
	if err := configor.Load(&cfg, *cfgFlag); err != nil {
		fmt.Println(err)
		return
	}
	cfg.AppendTo = lookForPath(cfg.AppendTo)
	cfg.KeysPath = strings.ReplaceAll(cfg.KeysPath, "%userprofile%", os.Getenv("userprofile"))
	cfg.SSHConfig = strings.ReplaceAll(cfg.SSHConfig, "%userprofile%", os.Getenv("userprofile"))
	historyPath = inExeDir(historyPath)
	switch {
	case *versionFlag:
		info()
	case *updateFlag, !fileExists(cfg.SSHConfig):
		update()
	case *vsdbgFlag:
		vsdbg()
	case *serverFlag:
		serverInfo()
	default:
		ssh()
	}
}
