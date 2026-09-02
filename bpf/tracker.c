// bpf/tracker.c
// CO-RE-free variant: reads the stable prefix of struct sock_common via
// bpf_probe_read_kernel instead of relying on kernel BTF at load time.
//
// Offset justification (verified against include/net/sock.h of the running
// kernel):
//   - struct sock's first member is `struct sock_common __sk_common` (offset 0)
// This intentionally targets the x86_64 layout used by the generated
// bindings. The installer compiles a native object on the target machine.
#include <linux/types.h>
#include <linux/bpf.h>
#include <linux/ptrace.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>
#include <bpf/bpf_tracing.h>

// Incomplete declaration only: we never dereference struct sock in BPF code.
// Needed at file scope so BPF_KPROBE's typeof() prototypes agree.
struct sock;

#define AF_INET 2
#define AF_INET6 10
#define IPPROTO_TCP 6
#define IPPROTO_UDP 17

struct sock_common_prefix {
    union {
        struct { __be32 skc_daddr; __be32 skc_rcv_saddr; } v4;
    } addr;
    __u32 skc_hash;
    union { struct { __be16 skc_dport; __u16 skc_num; }; __u32 skc_portpair; } ports;
    __u16 skc_family;
    __u8 skc_state;
    __u8 skc_flags;
    __u32 skc_bound_dev_if;
    __u64 skc_bind_node[2];
    void *skc_prot;
    void *skc_net;
    __u8 skc_v6_daddr[16];
};

char LICENSE[] SEC("license") = "GPL";

struct traffic_t {
    __u64 tx_bytes;
    __u64 rx_bytes;
};

struct key_t {
    __u32 pid;
    __u32 ifindex;
    __u16 port;
    __u16 family;
    __u8 protocol;
    __u8 pad[3];
    __u8 daddr[16];
};

struct sock_info_t { __u64 ptr; __u32 protocol; };

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, struct key_t);
    __type(value, struct traffic_t);
} traffic_map SEC(".maps");

// Keyed by thread id (needed to correlate kprobe <-> kretprobe in the same
// task); stores the struct sock pointer as a plain __u64.
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, __u32);
    __type(value, struct sock_info_t);
} sock_map SEC(".maps");

static __always_inline void update_traffic(struct key_t *key, __u64 tx, __u64 rx) {
    struct traffic_t *val = bpf_map_lookup_elem(&traffic_map, key);
    if (!val) {
        struct traffic_t new_val = { .tx_bytes = tx, .rx_bytes = rx };
        bpf_map_update_elem(&traffic_map, key, &new_val, BPF_ANY);
    } else {
        val->tx_bytes += tx;
        val->rx_bytes += rx;
    }
}

// struct sock_common starts at struct sock offset zero on supported kernels.
static __always_inline int read_socket(void *sk, struct key_t *key, __u32 protocol) {
    struct sock_common_prefix *sc = (struct sock_common_prefix *)sk;
    __u16 family = 0;
    __u16 port = 0;
    __u32 ifindex = 0;
    bpf_probe_read_kernel(&family, sizeof(family), &sc->skc_family);
    bpf_probe_read_kernel(&port, sizeof(port), &sc->ports.skc_dport);
    bpf_probe_read_kernel(&ifindex, sizeof(ifindex), &sc->skc_bound_dev_if);
    key->family = family;
    key->port = bpf_ntohs(port);
    key->ifindex = ifindex;
    key->protocol = protocol;
    if (family == AF_INET6) {
        bpf_probe_read_kernel(key->daddr, sizeof(key->daddr), sc->skc_v6_daddr);
    } else {
        __be32 daddr = 0;
        bpf_probe_read_kernel(&daddr, sizeof(daddr), &sc->addr.v4.skc_daddr);
        __builtin_memcpy(key->daddr, &daddr, sizeof(daddr));
    }
    return family == AF_INET || family == AF_INET6;
}

static __always_inline __u32 tgid(void) {
    return bpf_get_current_pid_tgid() >> 32;
}

static __always_inline __u32 tid(void) {
    return (__u32)bpf_get_current_pid_tgid();
}

static __always_inline void save_sock(void *sk, __u32 protocol) {
    struct sock_info_t info = { .ptr = (__u64)sk, .protocol = protocol };
    __u32 key = tid();
    bpf_map_update_elem(&sock_map, &key, &info, BPF_ANY);
}

// --- TCP SEND ---
SEC("kprobe/tcp_sendmsg")
int BPF_KPROBE(kprobe_tcp_sendmsg, struct sock *sk, void *msg, __u64 size) {
    struct key_t key = {};
    key.pid = tgid();
    if (read_socket(sk, &key, IPPROTO_TCP)) update_traffic(&key, size, 0);
    return 0;
}

// --- TCP RECV (entry: stash the sock pointer for the kretprobe) ---
SEC("kprobe/tcp_recvmsg")
int BPF_KPROBE(kprobe_tcp_recvmsg, struct sock *sk) {
    save_sock(sk, IPPROTO_TCP);
    return 0;
}

// --- TCP RECV (return) ---
SEC("kretprobe/tcp_recvmsg")
int BPF_KRETPROBE(kretprobe_tcp_recvmsg, int copied) {
    __u32 map_key = tid();
    if (copied <= 0) { bpf_map_delete_elem(&sock_map, &map_key); return 0; }
    struct sock_info_t *info = bpf_map_lookup_elem(&sock_map, &map_key);
    if (!info) return 0;

    struct key_t key = {};
    key.pid = tgid();
    if (read_socket((void *)info->ptr, &key, info->protocol)) update_traffic(&key, 0, copied);
    bpf_map_delete_elem(&sock_map, &map_key);
    return 0;
}

// --- UDP SEND ---
SEC("kprobe/udp_sendmsg")
int BPF_KPROBE(kprobe_udp_sendmsg, struct sock *sk, void *msg, __u64 size) {
    struct key_t key = {};
    key.pid = tgid();
    if (read_socket(sk, &key, IPPROTO_UDP)) update_traffic(&key, size, 0);
    return 0;
}

// --- UDP RECV (entry) ---
SEC("kprobe/udp_recvmsg")
int BPF_KPROBE(kprobe_udp_recvmsg, struct sock *sk) {
    save_sock(sk, IPPROTO_UDP);
    return 0;
}

// --- UDP RECV (return) ---
SEC("kretprobe/udp_recvmsg")
int BPF_KRETPROBE(kretprobe_udp_recvmsg, int copied) {
    __u32 map_key = tid();
    if (copied <= 0) { bpf_map_delete_elem(&sock_map, &map_key); return 0; }
    struct sock_info_t *info = bpf_map_lookup_elem(&sock_map, &map_key);
    if (!info) return 0;

    struct key_t key = {};
    key.pid = tgid();
    if (read_socket((void *)info->ptr, &key, info->protocol)) update_traffic(&key, 0, copied);
    bpf_map_delete_elem(&sock_map, &map_key);
    return 0;
}
