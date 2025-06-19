# Containerd Snapshotters

## 1. Introduction to Snapshotters

Snapshotters manage how container images are stored and accessed in containerd. They handle the copy-on-write (COW) operations that let containers share common image layers while keeping their changes separate.

In containerd, snapshotters:
- Manage image layers on disk
- Enable efficient sharing of common layers
- Mount filesystem views for containers
- Clean up unused layers

This design lets containerd work with different storage systems through various snapshotter implementations.

## 2. Built-in Snapshotters

### Overlayfs (Default)
The standard snapshotter that stacks image layers using the Linux kernel's overlay filesystem.

**Key points:**
- Uses read-only lower layers with a writable upper layer
- Works on most Linux distributions
- Good performance for most workloads
- May have issues with many layers (inode limits)

### Native
Uses basic file operations without special filesystem features.

**Key points:**
- Works on any filesystem
- Simple but inefficient (uses more disk space)
- Useful as a fallback option

### BTRFS
Uses BTRFS filesystem's built-in snapshot features.

**Key points:**
- Efficient native COW operations
- Fast snapshot creation
- Needs BTRFS filesystem

### ZFS
Leverages ZFS filesystem features for snapshots.

**Key points:**
- Strong data integrity through checksums
- Compression and deduplication features
- Needs ZFS filesystem support

### Device Mapper
Uses Linux device mapper for block-level snapshots.

**Key points:**
- Block-level rather than file-level operations
- Strong container isolation
- More complex setup than filesystem options

## 3. Plugin-based Snapshotters

Containerd can be extended with third-party snapshotters through its plugin system.

### Nydus
A FUSE-based snapshotter that optimizes image distribution and startup.

**Key points:**
- Loads data blocks on demand
- Significantly speeds up container startup
- Works with special registry backends
- Integrates through containerd's plugin interface

### Stargz
Enables starting containers before downloading complete images.

**Key points:**
- Uses eStargz format for random access to image parts
- Containers start with only partial image data
- Works with standard OCI registries
- Uses FUSE for implementation

## 4. Performance & Use Cases

### Performance Comparison

| Snapshotter | Startup | Disk Usage | Best Use Case |
|-------------|---------|------------|---------------|
| Overlayfs   | Fast    | Moderate   | General purpose |
| Native      | Slow    | High       | Simple compatibility |
| BTRFS       | Very Fast | Low      | Data-heavy workloads |
| ZFS         | Fast    | Low        | When ZFS is available |
| Device Mapper | Moderate | Low     | High-security needs |
| Nydus/Stargz | Very Fast* | Low*   | Limited bandwidth |

*For remote images with lazy loading

### Key Trade-offs

**Simplicity vs. Optimization:**
Built-in snapshotters are simpler but plugin options offer specialized features.

**Storage vs. Startup Speed:**
Traditional snapshotters need full image downloads while lazy-loading options start faster.

**Compatibility:**
Some snapshotters need specific filesystem types or kernel features.

## Summary

Choosing the right snapshotter affects container performance, storage efficiency, and startup times. For most uses, overlayfs balances performance and compatibility. For specific needs like large images or bandwidth constraints, plugin snapshotters like Nydus or stargz offer significant advantages through lazy loading.
