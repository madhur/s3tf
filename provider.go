package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	termbox "github.com/nsf/termbox-go"
)

type EventHandler interface {
	Handle(termbox.Event)
}

type ProviderStatus int

const (
	StateList ProviderStatus = iota //0
	StateMenu
	StateDetail
)

type Provider struct {
	EventHandler
	quitChan       chan struct{}
	status         ProviderStatus
	navigator      *Node
	bucket         string
	listView       *ListView
	navigationView *NavigationView
	statusView     *StatusView
	menuView       *MenuView
	detailView     *DetailView
}

func NewProvider() *Provider {
	p := &Provider{}
	p.Init()
	p.Update()
	p.Draw()
	return p
}

func (p *Provider) Init() {
	// Init s3 data structure
	rootNode := NewNode("", nil, ListBuckets())
	width, height := termbox.Size()
	halfWidth := width / 2
	halfHeight := height / 2

	p.status = StateList
	p.navigator = rootNode

	listView := &ListView{}
	listView.objects = p.navigator.objects
	listView.key = p.navigator.key
	listView.win = newWindow(0, 1, width, height-2)
	listView.cursorPos = newPosition(0, 0)
	listView.drawPos = newPosition(0, 0)
	p.listView = listView

	navigationView := &NavigationView{}
	navigationView.win = newWindow(0, 0, width, 1)
	p.navigationView = navigationView

	statusView := &StatusView{}
	statusView.win = newWindow(0, height-1, width, 1)
	p.statusView = statusView

	menuView := &MenuView{}
	menuView.items = []*MenuItem{
		NewMenuItem("download", "w", "download file.", CommandDownload),
		NewMenuItem("open", "o", "open file.", CommandOpen),
		NewMenuItem("edit", "e", "open editor by file.", CommandEdit),
	}
	menuView.layer = NewLayer(0, halfHeight, width, height-halfHeight)
	p.menuView = menuView

	detailView := &DetailView{}
	detailView.layer = NewLayer(halfWidth, 1, width-halfWidth, height-2)
	p.detailView = detailView
}

func (p *Provider) Loop() {
	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			p.Handle(ev)
			p.Update()
		case termbox.EventError:
			panic(ev.Err)
		case termbox.EventInterrupt:
			return
		}
		p.Draw()
	}
}

func (p *Provider) Update() {
	p.navigationView.SetKey(p.bucket, p.navigator.key)
}

func (p *Provider) Draw() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	defer termbox.Flush()
	p.listView.Draw()
	p.navigationView.Draw()
	if p.status == StateMenu {
		p.menuView.Draw()
	}
	if p.status == StateDetail {
		p.detailView.Draw()
	}
	p.statusView.Draw()
}

func (p *Provider) reload() {
	if p.navigator.IsRoot() {
		p.navigator.objects = ListBuckets()
		p.listView.objects = p.navigator.objects
		return
	}

	if p.navigator.IsBucketRoot() {
		p.navigator.objects = ListObjects(p.bucket, "")
		p.listView.objects = p.navigator.objects
		return
	}

	p.navigator.objects = ListObjects(p.bucket, p.navigator.key)
	p.listView.objects = p.navigator.objects
}

func (p *Provider) download() {
	obj := p.listView.getCursorObject()
	bucketName := p.bucket
	switch obj.ObjType {
	case Object:
		currentDir, _ := os.Getwd()
		f, err := os.Create(filepath.Join(currentDir, Filename(obj.Name)))
		if err != nil {
			log.Fatalf("failed create donwload reader, %v", err)
		}
		defer f.Close()

		DownloadObject(bucketName, obj.Name, f)
		path := "s3://" + strings.Join([]string{bucketName, obj.Name}, "/")
		p.statusView.msg = fmt.Sprintf("download complate. %s", path)
	default:
		log.Println("Invalid s3 object type")
	}
}

func (p *Provider) open() {
	obj := p.listView.getCursorObject()
	bucketName := p.bucket
	switch obj.ObjType {
	case Object:
		tempDir, _ := ioutil.TempDir("", "")
		f, err := os.Create(filepath.Join(tempDir, Filename(obj.Name)))
		if err != nil {
			log.Fatalf("failed create donwload reader, %v", err)
		}
		defer f.Close()

		DownloadObject(bucketName, obj.Name, f)
		if err := Open(f.Name()); err != nil {
			log.Fatalf("failed open file, %v", err)
		}

		path := "s3://" + strings.Join([]string{bucketName, obj.Name}, "/")
		p.statusView.msg = fmt.Sprintf("open. %s", path)
	default:
		log.Println("Invalid s3 object type")
	}
}

func (p *Provider) edit() {
	obj := p.listView.getCursorObject()
	bucketName := p.bucket
	switch obj.ObjType {
	case Object:
		tempDir, _ := ioutil.TempDir("", "")
		f, err := os.Create(filepath.Join(tempDir, Filename(obj.Name)))
		if err != nil {
			log.Fatalf("failed create donwload reader, %v", err)
		}
		DownloadObject(bucketName, obj.Name, f)
		editFilePath := f.Name()
		f.Close()

		termbox.Close()
		defer termbox.Init()
		OpenEditor(editFilePath)

		path := "s3://" + strings.Join([]string{bucketName, obj.Name}, "/")
		p.statusView.msg = fmt.Sprintf("edit. %s", path)
	default:
		log.Println("Invalid s3 object type")
	}
}

func (p *Provider) show(obj *S3Object) {
	switch obj.ObjType {
	case Bucket:
		bucketName := obj.Name
		p.bucket = bucketName
		if p.navigator.IsExistChildren(bucketName) {
			p.moveNext(bucketName)
			return
		}
		objects := ListObjects(bucketName, "")
		p.loadNext(bucketName, objects)
	case Dir:
		bucketName := p.bucket
		objectKey := obj.Name
		if p.navigator.IsExistChildren(objectKey) {
			p.moveNext(objectKey)
			return
		}
		objects := ListObjects(bucketName, objectKey)
		p.loadNext(objectKey, objects)
	case PreDir:
		p.loadPrev()
	case Object:
	default:
		log.Fatalln("Invalid s3 object type")
	}
}

func (p *Provider) moveNext(key string) {
	child := p.navigator.GetChild(key)
	p.navigator = child
	p.listView.updateList(child)
	log.Printf("Move next. child:%s", child.key)
}

func (p *Provider) loadNext(key string, objects []*S3Object) {
	parent := p.navigator
	child := NewNode(key, parent, objects)
	parent.AddChild(key, child)
	p.navigator = child
	p.listView.updateList(child)
	log.Printf("Load next. parent:%s, child:%s", parent.key, child.key)
}

func (p *Provider) loadPrev() {
	parent := p.navigator.parent
	p.navigator = parent
	p.listView.updateList(parent)
	log.Printf("Load prev. parent:%s", parent.key)
}

func (p *Provider) menu() {
	p.status = StateMenu
}

func (p *Provider) detail(obj *S3Object) {
	p.status = StateDetail
	p.detailView.obj = Detail(p.bucket, obj.Name)
	p.detailView.key = obj.Name
}

func (p *Provider) Handle(ev termbox.Event) {
	switch p.status {
	case StateList:
		p.listEvent(ev)
	case StateMenu:
		p.menuEvent(ev)
	case StateDetail:
		p.detailEvent(ev)
	}
}

func (p *Provider) listEvent(ev termbox.Event) {
	if ev.Key == termbox.KeyEsc || ev.Ch == 'q' {
		go func() {
			termbox.Interrupt()
			time.Sleep(1 * time.Second)
			panic("this should never run")
		}()
	} else if ev.Ch == 'j' || ev.Key == termbox.KeyArrowDown || ev.Key == termbox.KeyCtrlN {
		p.navigator.position = p.listView.down()
	} else if ev.Ch == 'k' || ev.Key == termbox.KeyArrowUp || ev.Key == termbox.KeyCtrlP {
		p.navigator.position = p.listView.up()
	} else if ev.Ch == 'm' {
		p.menu()
	} else if ev.Ch == 'd' {
		obj := p.listView.getCursorObject()
		p.detail(obj)
	} else if ev.Ch == 'h' || ev.Key == termbox.KeyArrowLeft {
		if !p.navigator.IsRoot() {
			p.loadPrev()
		}
	} else if ev.Ch == 'r' {
		p.reload()
	} else if ev.Ch == 'w' {
		p.download()
	} else if ev.Ch == 'o' {
		p.open()
	} else if ev.Ch == 'e' {
		p.edit()
	} else if ev.Ch == 'l' || ev.Key == termbox.KeyArrowRight || ev.Key == termbox.KeyEnter {
		obj := p.listView.getCursorObject()
		p.show(obj)
	}
}

func (p *Provider) menuEvent(ev termbox.Event) {
	if ev.Ch == 'j' || ev.Key == termbox.KeyArrowDown || ev.Key == termbox.KeyCtrlN {
		p.menuView.down()
	} else if ev.Ch == 'k' || ev.Key == termbox.KeyArrowUp || ev.Key == termbox.KeyCtrlP {
		p.menuView.up()
	} else if ev.Ch == 'q' {
		p.status = StateList
	} else if ev.Key == termbox.KeyEnter {
		item := p.menuView.getCursorItem()
		switch item.command {
		case CommandDownload:
			p.download()
		case CommandOpen:
			p.open()
		case CommandEdit:
			p.edit()
		}
		p.status = StateList
	}
}

func (p *Provider) detailEvent(ev termbox.Event) {
	if ev.Ch == 'j' || ev.Key == termbox.KeyArrowDown || ev.Key == termbox.KeyCtrlN {
		p.detailView.down()
	} else if ev.Ch == 'k' || ev.Key == termbox.KeyArrowUp || ev.Key == termbox.KeyCtrlP {
		p.detailView.up()
	} else if ev.Ch == 'q' {
		p.status = StateList
	}
}
