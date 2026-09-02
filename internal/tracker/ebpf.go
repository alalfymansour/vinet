// internal/tracker/ebpf.go
package tracker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"

	"github.com/alalfymansour/vinet/internal/db"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -D__TARGET_ARCH_x86" bpf ../../bpf/tracker.c

func StartEbpf(ctx context.Context, database *sql.DB) error {
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("remove memlock: %w", err)
	}

	var objs bpfObjects
	// Give the kernel verifier a large log buffer so rejections include the
	// full verifier log instead of being masked by EFAULT/ENOSPC truncation.
	collectionOpts := &ebpf.CollectionOptions{
		Programs: ebpf.ProgramOptions{LogSizeStart: 1 << 20},
	}
	if err := loadBpfObjects(&objs, collectionOpts); err != nil {
		var verr *ebpf.VerifierError
		if errors.As(err, &verr) {
			return fmt.Errorf("loading eBPF objects failed:\n%+v", verr)
		}
		return fmt.Errorf("loading eBPF objects failed: %w", err)
	}
	defer objs.Close()

	probes := []struct {
		name string
		prog *ebpf.Program
		ret  bool
	}{
		{"tcp_sendmsg", objs.KprobeTcpSendmsg, false},
		{"tcp_recvmsg", objs.KprobeTcpRecvmsg, false},
		{"tcp_recvmsg", objs.KretprobeTcpRecvmsg, true},
		{"udp_sendmsg", objs.KprobeUdpSendmsg, false},
		{"udp_recvmsg", objs.KprobeUdpRecvmsg, false},
		{"udp_recvmsg", objs.KretprobeUdpRecvmsg, true},
	}

	var links []link.Link
	for _, p := range probes {
		var l link.Link
		var err error
		if p.ret {
			l, err = link.Kretprobe(p.name, p.prog, nil)
		} else {
			l, err = link.Kprobe(p.name, p.prog, nil)
		}
		if err != nil {
			for _, existing := range links {
				_ = existing.Close()
			}
			return fmt.Errorf("attaching probe %s: %w", p.name, err)
		}
		links = append(links, l)
	}
	defer func() {
		for _, l := range links {
			l.Close()
		}
	}()

	log.Println("eBPF tracker attached (TCP + UDP + IP Tracking)!")

	go db.StartPruner(ctx, database)

	pollInterval := durationFromEnv("VINET_POLL_INTERVAL", 5*time.Second)
	// Poll once immediately. Waiting for the first ticker event makes a newly
	// started service look inactive and delays the first visible sample.
	if err := pollAndSaveEbpf(database, objs.TrafficMap); err != nil {
		log.Printf("initial poll: %v", err)
		setCollectorError(database, err)
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := pollAndSaveEbpf(database, objs.TrafficMap); err != nil {
				log.Printf("poll: %v", err)
				setCollectorError(database, err)
			}
		}
	}
}

func durationFromEnv(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < time.Second {
		log.Printf("invalid %s=%q; using %s", name, value, fallback)
		return fallback
	}
	return duration
}

func pollAndSaveEbpf(database *sql.DB, trafficMap *ebpf.Map) error {
	var key bpfKeyT
	var stats bpfTrafficT
	tx, err := database.Begin()
	if err != nil {
		return fmt.Errorf("begin traffic batch: %w", err)
	}
	stmt, err := tx.Prepare("INSERT INTO traffic (pid, process_name, executable_path, dest_ip, family, protocol, dest_port, ifindex, bytes_sent, bytes_recv) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare traffic batch: %w", err)
	}
	defer stmt.Close()
	var processed []bpfKeyT

	iter := trafficMap.Iterate()
	for iter.Next(&key, &stats) {
		if stats.TxBytes == 0 && stats.RxBytes == 0 {
			continue
		}

		comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", key.Pid))
		if err != nil {
			_ = trafficMap.Delete(key)
			continue
		}
		processName := strings.TrimSpace(string(comm))
		if processName == "" {
			_ = trafficMap.Delete(key)
			continue
		}

		ipStr := formatAddress(key.Family, key.Daddr)
		executable, _ := os.Readlink(fmt.Sprintf("/proc/%d/exe", key.Pid))

		if _, err = stmt.Exec(
			key.Pid, processName, executable, ipStr, key.Family, key.Protocol, key.Port, key.Ifindex, stats.TxBytes, stats.RxBytes,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert traffic: %w", err)
		}
		processed = append(processed, key)
	}
	if err := iter.Err(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("iterate traffic map: %w", err)
	}
	// Keep the heartbeat in the same transaction as the samples. This avoids
	// reporting a healthy collector when its traffic batch was not committed.
	if _, err := tx.Exec("INSERT INTO collector_state (id, last_seen, last_error) VALUES (1, CURRENT_TIMESTAMP, '') ON CONFLICT(id) DO UPDATE SET last_seen=excluded.last_seen, last_error='' "); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("update collector heartbeat: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit traffic batch: %w", err)
	}
	for _, key := range processed {
		if err := trafficMap.Delete(key); err != nil {
			log.Printf("delete map key: %v", err)
		}
	}
	return nil
}

func setCollectorError(database *sql.DB, pollErr error) {
	const query = `INSERT INTO collector_state (id, last_seen, last_error)
VALUES (1, COALESCE((SELECT last_seen FROM collector_state WHERE id=1), CURRENT_TIMESTAMP), ?)
ON CONFLICT(id) DO UPDATE SET last_error=excluded.last_error`
	if _, err := database.Exec(query, pollErr.Error()); err != nil {
		log.Printf("record collector error: %v", err)
	}
}

func formatAddress(family uint16, raw [16]uint8) string {
	if family == 2 {
		return net.IP(raw[:4]).String()
	}
	return net.IP(raw[:]).String()
}
