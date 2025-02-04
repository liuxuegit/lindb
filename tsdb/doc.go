package tsdb

/*


━━━━━━━━━━━━━━━━━━━━━━━━━━Layout of Shard━━━━━━━━━━━━━━━━━━━━━━━━

Each shard contains a MemoryDatabase and DiskDatabase.
┌──────────────────┬──────────────────┐
│       Shard      │       Shard      │
├──────────────────┼──────────────────┤
│       MemDB      │       MemDB      │
├──────────────────┼──────────────────┤
│       DiskDB     │       DiskDB     │
├──────────────────┴──────────────────┤
│               IndexDB               │
└─────────────────────────────────────┘


━━━━━━━━━━━━━━━━━━━━━━━━━━Layout of memDB━━━━━━━━━━━━━━━━━━━━━━━━

+--------------+       +--------------+
|              |------>|              |
|              |-+     |              |-+
|   Memory     | |     |  Metric      | |
|   Database   | |-+   |  Store       | |-+
|   RwMutex    | | |   |  RWMutex     | | |
|              | | |   |              | | |
|              | | |   |              | | |
|              | | |   |              | | |
|              | | |   |              | | |
|              | | |   |              | | |
+-+------------+ | |   +--------------+ | |
  +--------------+ |     +-----|--------+ |
    +--------------+       +---|----------+
                               |
                               V
+--------------+       +--------------+
|              |<------|              |
|              |-+     |              |-+
|              | |     |              | |
|   Field      | |-+   |  TimeSeries  | |-+
|   Store      | | |   |  Store       | | |
|              | | |   |              | | |
|              | | |   |              | | |
|              | | |   |              | | |
|              | | |   |              | | |
|              | | |   |   SpinLock   | | |
+--------------+ | |   +--------------+ | |
  +----|---------+ |     +--------------+ |
    +--|-----------+       +--------------+
       |
       V
+--------------+
|              |
|              |-+
|              | |
|   Segment    | |-+
|   Store      | | |
|              | | |
|              | | |
|              | | |
|              | | |
|              | | |
+--------------+ | |
  +--------------+ |
    +--------------+


━━━━━━━━━━━━━━━━━━━━━━━━━━Layout of series index table━━━━━━━━━━━━━━━━━━━━━━━━

                   Level1
                   +---------+---------+---------+---------+---------+---------+
                   |  TagKV  |  TagKV  |  TagKV  |  TagKV  |  TagKV  | Footer  |
                   | EntrySet| EntrySet| EntrySet| Offset  |  Index  |         |
                   +---------+---------+---------+---------+---------+---------+
                  /           \                   \        |\        +-------------------------------+
                 /             \                   \       | +--------------------------------+       \
                /               \                   \      +-----------------------------+     \       \
               /                 \                   +--------------+                     \     \       \
  +-----------+                   +--------------------------+       \                     \     \       \
 /                 Level2                                     \       \                     \     \       \
v--------+--------+--------+--------+--------+--------+--------v       v--------+---+--------v     v-------v
| LOUDS  |TagValue| Data1  | Data2  |TagValue|TagValue| CRC32  |       | Offset |...| Offset |     | TagKV |
|TrieTree| Count  | Length | Length | Data1  | Data2  |CheckSum|       |        |   |        |     | Bitmap|
+--------+--------+--------+--------+--------+--------+--------+       +--------+---+--------+     +-------+


Level1(KV table: TagKV EntrySet, Offset, Keys)
Level1 is same as metric-table as below
Key: tagID


Level2(LOUDS Encoded Trie Tree)
┌─────────────────────────────────────────────────────────────────┐
│                    LOUDS Encoded Trie Tree                      │
├──────────┬──────────┬──────────┬──────────┬──────────┬──────────┤
│  Labels  │  labels  │ isPrefix │ isPrefix │  LOUDS   │  LOUDS   │
│  Length  │  Block   │ Key Len  │Key BitMap│  Length  │  BitMap  │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│ uvariant │ N Bytes  │ uvariant │ N Bytes  │ uvariant │ N Bytes  │
└──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘

Level2(Versioned TagValue Data)
┌────────────────────────────────────────────────────────────────────────────┐
│                      Versioned TagValue Data                               │
├──────────┬──────────┬──────────┬──────────┬──────────┬──────────┬──────────┤
│ Version  │ Version1 │ Version2 │ Version1 │ Version2 │TagValue1 │TagValue2 │
│  Count   │ (Delta)  │ (Delta)  │  Length  │  Length  │  BitMap  │  BitMap  │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│ uvariant │ 4 Bytes  │ uvariant │ uvariant │ uvariant │ N Bytes  │  N Bytes │
└──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘

Succinct trie tree(Example):
(KEY Value: eleme:1, etcd:2, etrace:3)

Labels: eltecrmdaece
isPrefixKey: 0000000010101
LOUDS: 1011010110101010100100100
Values: [2, 1, 3]


                   +--------+
                   |        | (pseudo root)
                   |  10    |
                   +--------+
                       |
                   +---v----+
                   |   e    |
                   |  110   |
                   +---+----+
                      / \
              +------+   +----+
             /                 \
        +---v----+          +---v----+
        |   l    |          |   t    |
        |   10   |          |   110  |
        +---+----+          +---+----+
            |                   |\_______________
            |                   |                \
        +---v----+          +---v----+        +---v----+
        |   e    |          |   c    |        |   r    |
        |   10   |          |   10   |        |   10   |
        +---+----+          +---+----+        +---+----+
            |                   |                 |
        +---v----+          +---v----+        +---v----+
        |   m    |          |   d    |        |   a    |
        |   10   |          |   0    |        |   10   |
        +---+----+          +--------+        +---+----+
            |                 Value:2             |
        +---v----+                            +---v----+
        |   e    |                            |   c    |
        |   0    |                            |   10   |
        +--------+                            +---+----+
          Value:1                                 |
                                              +---v----+
                                              |   e    |
                                              |   0    |
                                              +--------+
                                               Value:3


━━━━━━━━━━━━━━━━━━━━━━━━━━Layout of metric index table━━━━━━━━━━━━━━━━━━━━━━━━
Metric Index table is composed of 2 parts: Metric Names and Metric Meta:

a) Metric-NameID-Table
Metric-NameID-Table is a gzip compressed k/v pairs of metricNames and metricIDs on disk.

                   Level1
                   +---------+---------+---------+---------+
                   | Metric  |  Meta   | Index   | Footer  |
                   | KVPair  |         |         |         |
                   +---------+---------+---------+---------+

Level1(Metric NameID Table)
┌─────────────────────────────────────────────────────────────────┬─────────────────────┐
│            Gzip Compressed Metric K/V pairs                     │  SequenceNumber     │
├──────────┬──────────┬──────────┬──────────┬──────────┬──────────┼──────────┬──────────┤
│MetricName│MetricName│ MetricID │MetricName│MetricName│ MetricID │ MetricID │  TagID   │
│  Length  │          │          │  Length  │          │          │ Sequence │ Sequence │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│ uvariant │ N Bytes  │ 4 Bytes  │ uvariant │ N Bytes  │ 4 Bytes  │ 4 Bytes  │ 4 Bytes  │
└──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘


b) Metric Meta Table
Metric-Meta stores meta info for metric,
such as tagKey, tagID, fieldID, fieldName and fieldType etc.

                   Level1
                   +---------+---------+---------+---------+---------+---------+
                   | Metric  | Metric  | Metric  | Metric  | Metric  | Footer  |
                   | Meta    |  Meta   |  Meta   |  Meta   | Index   |         |
                   +---------+---------+---------+---------+---------+---------+
                  /           \        |         |\        +--------------+
                 /             \       +         | +---------------+       \
                /               \       \        +------------+     \       \
               /                 \       \                     \     \       \
  +-----------+                   \       \                     \     \       \
 /                 Level2          \       \                     \     \       \
v--------+--------+--------+--------v       v--------+---+--------v     v-------v
|  Tag   | TagKey | Field  | Field  |       | Offset |...| Offset |     | Metric|
| MetaLen|  Meta  | MetaLen| Meta   |       |        |   |        |     | Bitmap|
+--------+--------+--------+--------+       +--------+---+--------+     +-------+

Level2(TagKey Meta)
┌──────────┬─────────────────────────────────────────────────────────────────┐
│  MetaLen │                       TagKey Meta                               │
├──────────┼──────────┬──────────┬──────────┬──────────┬──────────┬──────────┤
│  TagKey  │  TagKey  │  TagKey  │  TagID   │  TagKey  │  TagKey  │  TagID   │
│  MetaLen │   Len    │          │          │   Len    │          │          │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│ uvariant │  1 Byte  │ N Bytes  │ 4 Bytes  │  1 Byte  │ N Bytes  │ 4 Bytes  │
└──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘

Level2(Field Meta)
┌──────────┬───────────────────────────────────────────────────────────────────────────────────────┐
│  MetaLen │                                    Field Meta                                         │
├──────────┼──────────┬──────────┬──────────┬──────────┬──────────┬──────────┬──────────┬──────────┤
│  Field   │  Field   │  Field   │  Field   │  Field   │  Field   │  Field   │  Field   │  Field   │
│  MetaLen │   Len    │  Name    │  Type    │   ID     │   Len    │  Name    │  Type    │   ID     │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│ uvariant │  1 Byte  │ N Bytes  │ 1 Byte   │ 2 Bytes  │  1 Byte  │ N Bytes  │  1 Byte  │ 2 Bytes  │
└──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘


━━━━━━━━━━━━━━━━━━━━━━━━━━Layout of metric table━━━━━━━━━━━━━━━━━━━━━━━━

                   Level1
                   +---------+---------+---------+---------+---------+---------+
                   | Metric  | Metric  | Metric  | Metric  | Metric  | Footer  |
                   | Block   | Block   | Block   | Offset  | Index   |         |
                   +---------+---------+---------+---------+---------+---------+
                  /           \                   \        |\        +-------------------------------+
                 /             \                   \       | +--------------------------------+       \
                /               \                   \      +-----------------------------+     \       \
               /                 \                   +--------------+                     \     \       \
  +-----------+                   +--------------------------+       \                     \     \       \
 /                 Level2                                     \       \                     \     \       \
v--------+--------+-----------------+--------+--------+--------v       v--------+---+--------v     v-------v
| Series | Series | Series | Series | Series | Fields | Footer |       | Offset |...| Offset |     | Metric|
| Entry  | Entry  | Entry  | Offset | Index  |  Meta  |        |       |        |   |        |     | Bitmap|
+--------+--------+--------+--------+--------+--------+--------+       +--------+---+--------+     +-------+
|         \                 \       |\        \
|          \                 \      | \        +-----------------------------------------------+
|           \                 \     |  +----------------------------------------------+         \
|            \                 \    +---------------------------------------------+    \         \
|             \                 +-----------------------------+                    \    \         \
|              +------------------------------------+          \                    \    \         \
|                  Level3                            \          \                    \    \         \
v--------+--------+--------+--------+--------+--------v          v--------+---+-------v    v---------v
| Fields | Data   |  Data  | Data   | Data   |  Data  |          | Offset |...| Offset|    |seriesID |
| Info   |        |        |        |        |        |          |        |   |       |    | Bitmap  |
+--------+--------+--------+--------+--------+--------+          +--------+---+-------+    +---------+


Level1(KV table: MetricBlocks, Offset, Keys)
┌───────────────────────────────────────────┬───────────────────────────────────────────┐
│               Metric Blocks               │           Offset And Keys                 │
├──────────┬──────────┬──────────┬──────────┼──────────┬──────────┬──────────┬──────────┤
│  length  │  Metric  │  length  │  Metric  │  length  │  Offset  │  length  │  Keys    │
│          │  Block1  │          │  Block2  │          │          │          │          │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│ uvariant │  N Bytes │ uvariant │ N Bytes  │ uvariant │  N Bytes │ uvariant │  N Bytes │
└──────────┴──────────┴──────────┴──────────^──────────┴──────────^──────────┴──────────┘
                                            |                     |
                                       posOfOffset             posOfKeys

Level1(KV table: Footer)
┌──────────────────────────────────────────────────────┐
│                    Footer                            │
├──────────┬──────────┬──────────┬──────────┬──────────┤
│  length  │ position │ position │ Version  │  Magic   │
│          │ OfOffset │ OfKeys   │          │  Number  │
├──────────┼──────────┼──────────┼──────────┼──────────┤
│  1 Byte  │ 4 Bytes  │ 4 Bytes  │ 1 Bytes  │  8 Bytes │
└──────────┴──────────┴──────────┴──────────┴──────────┘



Level2(Fields Meta, Fields Footer)
┌─────────────────────────────────────────────────────────────────┬───────────────────────────────────────────┐
│               Fields Meta                                       │           Fields Footer                   │
├──────────┬──────────┬──────────┬──────────┬──────────┬──────────┼──────────┬──────────┬──────────┬──────────┤
│StartTime │ EndTime  │ Count    │ FieldID  │  Field   │          │ OffsetOf │ OffsetOf │ OffsetOf │  CRC32   │
│ (delta)  │ (delta)  │          │ (uint16) │  Type    │  ......  │ TSOffset │ TSIndex  │FieldsMeta│ Checksum │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│ uvariant │ uvariant │ uvariant │ 2 Bytes  │ 1 Byte   │ 3N Bytes │ 4 Bytes  │ 4 Bytes  │ 4 Bytes  │  4 Bytes │
└──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘


Level3(Fields Info, Fields Data)
┌─────────────────────────────────────────────────────────────────┬─────────────────────┐
│               Fields Info                                       │   Fields Data       │
├──────────┬──────────┬──────────┬──────────┬──────────┬──────────┼──────────┬──────────┤
│StartTime │ EndTime  │ BitArray │ BitArray │  Data1   │  Data2   │  Data1   │ Data2    │
│ (delta)  │ (delta)  │  Length  │          │  Length  │  Length  │          │          │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│ uvariant │ uvariant │ uvariant │ N Bytes  │ uvariant │ uvariant │ N Bytes  │ N Bytes  │
└──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘
bit array example(10101001, 1010100110101001)


*/
