package core

type CtrlStatus struct {
	Started bool
	Ready   bool
	Done    bool

	ReadyChan chan bool
	DoneChan  chan bool
}

func NewCtrlStatus() *CtrlStatus {
	return &CtrlStatus{
		ReadyChan: make(chan bool),
		DoneChan:  make(chan bool),
	}
}
