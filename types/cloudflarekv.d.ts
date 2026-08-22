type KVValueType = "text" | "json" | "arrayBuffer" | "stream";

interface KVNamespaceGetOptions<Type extends KVValueType = "text"> {
  type?: Type;
  cacheTtl?: number;
}

interface KVNamespacePutOptions {
  expiration?: number;
  expirationTtl?: number;
  metadata?: unknown;
}

interface KVNamespaceListOptions {
  prefix?: string;
  limit?: number;
  cursor?: string;
}

interface KVNamespaceListKey<Metadata = unknown> {
  name: string;
  expiration?: number;
  metadata?: Metadata;
}

interface KVNamespaceListResult<Metadata = unknown> {
  keys: Array<KVNamespaceListKey<Metadata>>;
  list_complete: boolean;
  cursor?: string;
}

type KVValueFor<Type extends KVValueType, Value = unknown> =
  Type extends "json" ? Value :
  Type extends "arrayBuffer" ? ArrayBuffer :
  Type extends "stream" ? ReadableStream<Uint8Array> : string;

interface KVNamespace {
  get<Value = unknown>(key: string, type?: "json"): Promise<Value | null>;
  get<Type extends KVValueType>(key: string, options: Type | KVNamespaceGetOptions<Type>): Promise<KVValueFor<Type> | null>;
  get(keys: string[], type?: "text" | KVNamespaceGetOptions<"text">): Promise<Map<string, string | null>>;
  get<Value = unknown>(keys: string[], type: "json" | KVNamespaceGetOptions<"json">): Promise<Map<string, Value | null>>;

  getWithMetadata<Value = unknown, Metadata = unknown>(key: string, type?: "json"): Promise<{ value: Value | null; metadata: Metadata | null }>;
  getWithMetadata<Type extends KVValueType, Metadata = unknown>(key: string, options: Type | KVNamespaceGetOptions<Type>): Promise<{ value: KVValueFor<Type> | null; metadata: Metadata | null }>;
  getWithMetadata<Metadata = unknown>(keys: string[], type?: "text" | KVNamespaceGetOptions<"text">): Promise<Map<string, { value: string | null; metadata: Metadata | null }>>;
  getWithMetadata<Value = unknown, Metadata = unknown>(keys: string[], type: "json" | KVNamespaceGetOptions<"json">): Promise<Map<string, { value: Value | null; metadata: Metadata | null }>>;

  put(key: string, value: string | ArrayBuffer | ArrayBufferView | ReadableStream<ArrayBuffer | ArrayBufferView>, options?: KVNamespacePutOptions): Promise<void>;
  delete(key: string): Promise<void>;
  list<Metadata = unknown>(options?: KVNamespaceListOptions): Promise<KVNamespaceListResult<Metadata>>;
}

interface SyncKVNamespace {
  get<Value = unknown>(key: string, type?: "json"): Value | null;
  get<Type extends Exclude<KVValueType, "stream">>(key: string, options: Type | KVNamespaceGetOptions<Type>): KVValueFor<Type> | null;
  getWithMetadata<Type extends Exclude<KVValueType, "stream">, Metadata = unknown>(key: string, options?: Type | KVNamespaceGetOptions<Type>): { value: KVValueFor<Type> | null; metadata: Metadata | null };
  put(key: string, value: string | ArrayBuffer | ArrayBufferView, options?: KVNamespacePutOptions): void;
  delete(key: string): void;
  list<Metadata = unknown>(options?: KVNamespaceListOptions): KVNamespaceListResult<Metadata>;
}

declare var KVNamespace: {
  new(namespace: string): KVNamespace;
};

declare var KV: KVNamespace;
declare var SyncKV: SyncKVNamespace;
