package ng

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/getevo/evo-ng"
	"github.com/getevo/evo-ng/internal/ds"
	"github.com/getevo/evo-ng/internal/file"
	"github.com/getevo/evo-ng/internal/proc"
	"github.com/moznion/gowrtr/generator"
	"os"
	"os/exec"
	"strings"
)

var skeleton Skeleton

var main = Main{
	Events: *ds.NewOrderedMap(),
}
var ContextInterface = Function{
	Name: "Extend",
	Extend: Type{
		IsPtr: true,
	},
	Result: []Type{
		{Struct: "error"},
	},
	Input: []Type{
		{
			IsPtr:  true,
			Pkg:    "evo",
			Struct: "Context",
		},
	},
}

func Start() {
	os.Remove("./go.mod")
	skeleton = GetSkeleton("./app.json")

	main.Events.Set("OverRide", generator.NewFunc(nil, generator.NewFuncSignature("OverRide")))
	main.Events.Set("Register", generator.NewFunc(nil, generator.NewFuncSignature("Register")))
	main.Events.Set("Ready", generator.NewFunc(nil, generator.NewFuncSignature("Ready")))
	main.Events.Set("Router", generator.NewFunc(nil, generator.NewFuncSignature("Router")))

	main.Root = generator.NewRoot()
	main.Main = generator.NewFunc(
		nil,
		generator.NewFuncSignature("main"),
	)

	main.Main = main.Main.AddStatements(
		generator.NewComment("Register EVO"),
		generator.NewRawStatement(`evo.Engine()`),
		generator.NewNewline(),
	)
	if !file.IsFileExist("./go.mod") {
		run("go", "mod", "init")
	}

	for src, dst := range skeleton.Replace {
		run("go", "mod", "edit", "-replace", src+"="+dst)
	}
	f, err := os.Open(file.WorkingDir() + "/go.mod")
	if err != nil {
		proc.Die("unable to open go.mod")
	}
	CopyModule("github.com/getevo/evo-ng")
	r := bufio.NewReader(f)
	line, _, err := r.ReadLine()
	if err != nil {
		panic(err)
	}
	skeleton.Module = string(bytes.TrimPrefix(line, []byte("module ")))

	skeleton.GenContext()

	var imports = []string{
		"github.com/getevo/evo-ng",
		skeleton.Module + "/http",
	}

	main.Main = main.Main.AddStatements(generator.NewRawStatement("evo.UseContext(&http.Context{})"))
	var imported = map[string]bool{}
	for _, include := range skeleton.Include {
		for _, event := range main.Events.Keys() {
			var pkg = skeleton.GetPackage(include)
			if pkg.HasFunction(Function{Name: fmt.Sprint(event), Result: []Type{{Struct: "error"}}}) {
				fn, _ := main.Events.Get(event)
				fn = fn.(*generator.Func).AddStatements(
					generator.NewRawStatement("evo.Register(" + pkg.Name + `.` + fmt.Sprint(event) + `)`),
					//generator.NewNewline(),
				)
				main.Events.Set(event, fn)
				if _, ok := imported[include]; !ok {
					if pkg.IsLocal {
						imports = append(imports, skeleton.Module+"/"+include)
					} else {
						imports = append(imports, strings.Split(include, "@")[0])
					}
					imported[include] = true
				}
			}
		}

	}

	main.Main = main.Main.AddStatements(
		generator.NewRawStatement("OverRide()"),
		generator.NewRawStatement("Register()"),
		generator.NewRawStatement("Router()"),
	)

	main.Main = main.Main.AddStatements(generator.NewRawStatement("evo.Run(Ready)"))

	main.Root = main.Root.AddStatements(
		generator.NewPackage("main"),
		generator.NewComment("GENERATED BY EVO-NG"),
		generator.NewImport(imports...),
		generator.NewNewline(),
	).AddStatements(main.Main)
	for _, event := range main.Events.Keys() {
		fn, _ := main.Events.Get(event)
		main.Root = main.Root.AddStatements(fn.(*generator.Func))
	}
	generated, err := main.Root.Generate(0)
	if err != nil {
		evo.Panic(err)
	}
	file.Write(file.WorkingDir()+"/main.go", generated)

	run("go", "mod", "vendor")

	Watcher()
}

func run(cmd string, args ...string) {
	fmt.Println(cmd, strings.Join(args, " "))
	c := exec.Command(cmd, args...)
	c.Dir = file.WorkingDir()
	c.Run()
}
