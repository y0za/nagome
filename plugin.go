package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
)

const (
	pluginFlashWaitDu time.Duration = 50 * time.Millisecond

	pluginMethodTCP           string = "tcp"
	pluginMethodStd                  = "std"
	pluginEachMessageChanSize        = 3
)

type plugin struct {
	Name        string   `yaml:"name"        json:"name"`
	Description string   `yaml:"description" json:"description"`
	Version     string   `yaml:"version"     json:"version"`
	Author      string   `yaml:"author"      json:"author"`
	Method      string   `yaml:"method"      json:"method"`
	Exec        []string `yaml:"exec"        json:"-"`
	Nagomever   string   `yaml:"nagomever"   json:"-"`
	Depends     []string `yaml:"depends"     json:"depends"`
	No          int      `yaml:"-"           json:"no"`
	rwc         io.ReadWriteCloser
	flushTm     *time.Timer
	wg          sync.WaitGroup
	cv          *CommentViewer
	setEnable   chan (bool)
	quit        chan (struct{})
	isEnable    bool
	writec      chan ([]byte)
	openc       chan (struct{})
}

// NewPlugin makes new Plugin.
func newPlugin(cv *CommentViewer) *plugin {
	return &plugin{
		openc:     make(chan struct{}),
		quit:      make(chan struct{}),
		setEnable: make(chan bool),
		writec:    make(chan []byte, pluginEachMessageChanSize),
		cv:        cv,
	}
}

func (pl *plugin) Open(rwc io.ReadWriteCloser) error {
	if pl.No == 0 {
		return fmt.Errorf("plugin \"%s\" is not initialized (add to CommentViewer)\n", pl.Name)
	}
	if pl.Name == "" {
		return fmt.Errorf("plugin \"%s\" no name is set\n", pl.Name)
	}
	if rwc == nil {
		return fmt.Errorf("given rw is nil\n")
	}
	if pl.rwc != nil {
		return fmt.Errorf("already opened\n")
	}

	pl.rwc = rwc
	pl.flushTm = time.NewTimer(time.Minute)

	pl.wg.Add(1)
	go pl.rwRoutine()

	close(pl.openc)
	pl.SetEnable(true)

	return nil
}

func (pl *plugin) SetEnable(e bool) {
	select {
	default:
		return
	case <-pl.openc:
	}
	select {
	case pl.setEnable <- e:
	case <-pl.quit:
	}
}

func (pl *plugin) WriteMess(m *Message) (fail bool) {
	jm, err := json.Marshal(m)
	if err != nil {
		log.Println(err)
		log.Println(m)
		return
	}
	return pl.Write(jm)
}

func (pl *plugin) Write(p []byte) (fail bool) {
	select {
	default:
		return true
	case <-pl.openc:
	}
	select {
	case pl.writec <- p:
		return false
	case <-pl.quit:
	}
	return true
}

func (pl *plugin) Depend(pln string) bool {
	f := false
	for _, d := range pl.Depends {
		if d == pln {
			f = true
			break
		}
	}
	return f
}

func (pl *plugin) Load(filePath string) error {
	d, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(d, pl)
	if err != nil {
		return err
	}

	return nil
}

func (pl *plugin) Save(filePath string) error {
	d, err := yaml.Marshal(pl)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filePath, d, 0600)
	if err != nil {
		return err
	}

	return nil
}

func (pl *plugin) IsMain() bool {
	return pl.No == 1
}

func (pl *plugin) rwRoutine() {
	defer pl.wg.Done()
	defer pl.rwc.Close()
	defer log.Printf("plugin [%s] is closing", pl.Name)

	// Run decoder.  It puts a message into "mes".
	dec := json.NewDecoder(pl.rwc)
	mes := make(chan (*Message))
	pl.wg.Add(1)
	go func() {
		defer pl.wg.Done()
		for {
			m := new(Message)
			err := dec.Decode(m)
			if err != nil {
				select {
				// ignore if quitting
				case <-pl.quit:
				default:
					if err != io.EOF {
						pl.cv.CreateEvNewDialog(CtUIDialogTypeInfo, "plugin disconnected",
							fmt.Sprintf("plugin [%s] : connection disconnected", pl.Name))
						log.Println(err)
					}
				}
				m = nil
			} else {
				m.prgno = pl.No
			}

			select {
			case mes <- m:
				if m == nil {
					return
				}
			case <-pl.quit:
				return
			}
		}
	}()

	bufw := bufio.NewWriter(pl.rwc)
	writeMess := func(p []byte) {
		pl.flushTm.Reset(pluginFlashWaitDu)
		_, err := fmt.Fprintf(bufw, "%s\n", p)
		if err != nil {
			log.Println(err)
			pl.cv.CreateEvNewDialog(CtUIDialogTypeInfo, "plugin", "failed to write a message : "+pl.Name)
			// quit if UI plugin disconnect
			if pl.IsMain() {
				pl.cv.Quit()
			} else {
				pl.close()
			}
		}
		return
	}
	for {
		select {
		// Process a received message
		case m := <-mes:
			if m == nil {
				// quit if UI plugin disconnect
				if pl.IsMain() {
					pl.cv.Quit()
				} else {
					pl.close()
				}
				continue
			}

			// ignore if plugin is not enabled
			if pl.isEnable {
				log.Printf("plugin message [%s] : %v", pl.Name, m)
				pl.cv.Evch <- m
			}

		// Send a message
		case m := <-pl.writec:
			if pl.isEnable == false {
				continue
			}
			writeMess(m)

		// Flush plugin IO
		case <-pl.flushTm.C:
			bufw.Flush()

		case e := <-pl.setEnable:
			if pl.isEnable == e {
				continue
			}
			pl.isEnable = e

			// send message
			m := &Message{
				Domain: DomainDirectngm,
			}
			if e == true {
				m.Command = CommDirectngmPlugEnabled
			} else {
				m.Command = CommDirectngmPlugDisabled
			}
			jm, err := json.Marshal(m)
			if err != nil {
				log.Println(err)
				log.Println(m)
				return
			}
			writeMess(jm)

		case <-pl.quit:
			return
		}
	}
}

func sendPluginMessage(cv *CommentViewer) {
	defer cv.wg.Done()

	for {
	readLoop:
		select {
		case mes := <-cv.Evch:
			// Direct
			if mes.Domain == DomainDirectngm {
				cv.Pgns[mes.prgno].WriteMess(mes)
				continue
			}
			if mes.Domain == DomainDirect {
				go func() {
					nicoerr := processDirectMessage(cv, mes)
					if nicoerr != nil {
						log.Printf("plugin message error form [%s] : %s\n", cv.Pgns[mes.prgno].Name, nicoerr)
						log.Println(mes)
					}
				}()
				continue
			}

			// filter

			// Messages from filter plugin will not send same plugin.
			var st int
			if strings.HasSuffix(mes.Domain, DomainSuffixFilter) {
				st = mes.prgno + 1
				mes.Domain = strings.TrimSuffix(mes.Domain, DomainSuffixFilter)
			}
			for i := st; i < len(cv.Pgns); i++ {
				if cv.Pgns[i].Depend(mes.Domain + DomainSuffixFilter) {
					// Add suffix to a message for filter plugin.
					tmes := *mes
					tmes.Domain = mes.Domain + DomainSuffixFilter
					fail := cv.Pgns[i].WriteMess(&tmes)
					if fail {
						continue
					}
					break readLoop
				}
			}

			jmes, err := json.Marshal(mes)
			if err != nil {
				log.Println(err)
				log.Println(mes)
				continue
			}

			// regular
			for i := range cv.Pgns {
				if cv.Pgns[i].Depend(mes.Domain) {
					cv.Pgns[i].Write(jmes)
				}
			}

			go func() {
				nicoerr := processPluginMessage(cv, mes)
				if nicoerr != nil {
					log.Printf("plugin message error form [%s] : %s\n", cv.Pgns[mes.prgno].Name, nicoerr)
					log.Println(mes)
				}
			}()

		case <-cv.quit:
			return
		}
	}
}

func handleTCPPlugin(c io.ReadWriteCloser, cv *CommentViewer) {
	defer cv.wg.Done()

	endc := make(chan bool, 1)

	cv.wg.Add(1)
	go func() {
		defer cv.wg.Done()
		select {
		case <-cv.quit:
			c.Close()
		case iserr := <-endc:
			if iserr {
				c.Close()
			}
		}
	}()

	dec := json.NewDecoder(c)

	m := new(Message)
	err := dec.Decode(m)
	if err != nil {
		log.Println(err)
		endc <- true
		return
	}
	if m.Domain != DomainDirect || m.Command != CommDirectNo {
		log.Println("send Direct.No message at first")
		endc <- true
		return
	}

	var ct CtDirectNo
	if err := json.Unmarshal(m.Content, &ct); err != nil {
		log.Println(err)
		endc <- true
		return
	}

	n := ct.No - 1
	if n < 0 || n >= len(cv.Pgns) {
		log.Println("received invalid plugin No.")
		endc <- true
		return
	}
	err = cv.Pgns[n].Open(c)
	if err != nil {
		log.Println(err)
		endc <- true
		return
	}
	log.Printf("loaded plugin : %s\n", cv.Pgns[n].Name)
	endc <- false
}

func handleSTDPlugin(p *plugin, cv *CommentViewer) {
	defer cv.wg.Done()

	if len(p.Exec) < 1 {
		log.Printf("exec is not specified in plugin [%s]\n", p.Name)
		return
	}

	cmd := exec.Command(p.Exec[0], p.Exec[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Println(err)
		return
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Println(err)
		return
	}
	defer stdout.Close()
	err = cmd.Start()
	if err != nil {
		log.Println(err)
		return
	}

	c := &stdReadWriteCloser{stdout, stdin}
	p.Open(c)
	log.Println("loaded plugin ", p)

	<-cv.quit
}

// Close closes opened plugin.
func (pl *plugin) Close() {
	pl.close()
	pl.wg.Wait()
}

func (pl *plugin) close() {
	close(pl.quit)
}

type stdReadWriteCloser struct {
	io.ReadCloser
	io.WriteCloser
}

func (rwc *stdReadWriteCloser) Close() error {
	errr := rwc.ReadCloser.Close()
	errw := rwc.WriteCloser.Close()
	if errr != nil {
		return errr
	}
	if errw != nil {
		return errw
	}
	return nil
}
