package payments

import (
	"context"
	"testing"
	"time"
)

func TestPermitCoordinatorBlocksSameKeyUntilRelease(t *testing.T) {
	c := newPermitCoordinator()
	key := permitKey{chainID: 314159, token: tokenAddr, owner: otherAddr}

	release, err := c.acquire(context.Background(), key)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	acquired := make(chan error, 1)
	go func() {
		releaseSecond, err := c.acquire(context.Background(), key)
		if err == nil {
			releaseSecond()
		}
		acquired <- err
	}()

	select {
	case err := <-acquired:
		t.Fatalf("second acquire completed before release: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	release()

	select {
	case err := <-acquired:
		if err != nil {
			t.Fatalf("second acquire: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second acquire did not complete after release")
	}
}

func TestPermitCoordinatorAllowsDifferentKeys(t *testing.T) {
	c := newPermitCoordinator()
	keyA := permitKey{chainID: 314159, token: tokenAddr, owner: otherAddr}
	keyB := permitKey{chainID: 314159, token: operatorAddr, owner: otherAddr}

	releaseA, err := c.acquire(context.Background(), keyA)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer releaseA()

	releaseB, err := c.acquire(context.Background(), keyB)
	if err != nil {
		t.Fatalf("different key acquire: %v", err)
	}
	releaseB()
}

func TestPermitCoordinatorAcquireContextCancel(t *testing.T) {
	c := newPermitCoordinator()
	key := permitKey{chainID: 314159, token: tokenAddr, owner: otherAddr}

	release, err := c.acquire(context.Background(), key)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.acquire(ctx, key); err == nil {
		t.Fatal("acquire err=nil, want context cancellation")
	}
}

func TestPermitCoordinatorReleaseIdempotent(t *testing.T) {
	c := newPermitCoordinator()
	key := permitKey{chainID: 314159, token: tokenAddr, owner: otherAddr}

	release, err := c.acquire(context.Background(), key)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	release()
	release()

	releaseAgain, err := c.acquire(context.Background(), key)
	if err != nil {
		t.Fatalf("reacquire after repeated release: %v", err)
	}
	releaseAgain()
}
