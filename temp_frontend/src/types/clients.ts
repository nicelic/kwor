import RandomUtil from "@/plugins/randomUtil"

type ClientConfigNamespace = "default" | "mihomo"
type ConfigFieldLocation = {
  key: string
  field: string
}
type AutoNamedFieldLocation = {
  key: string
  field: string
  aliases?: string[]
}

const sessionGeneratedUUIDs = new Set<string>()
const defaultUUIDStyleFields: ReadonlyArray<ConfigFieldLocation> = [
  { key: "vmess", field: "uuid" },
  { key: "vless", field: "uuid" },
  { key: "anytls", field: "password" },
  { key: "mieru", field: "password" },
  { key: "trojan", field: "password" },
  { key: "naive", field: "password" },
  { key: "hysteria", field: "auth_str" },
  { key: "tuic", field: "uuid" },
  { key: "tuic", field: "password" },
  { key: "hysteria2", field: "password" },
]
const mihomoUUIDStyleFields: ReadonlyArray<ConfigFieldLocation> = [
  { key: "vmess", field: "uuid" },
  { key: "vless", field: "uuid" },
  { key: "anytls", field: "password" },
  { key: "mieru", field: "password" },
  { key: "snell", field: "psk" },
  { key: "sudoku", field: "uuid" },
  { key: "trojan", field: "password" },
  { key: "tuic", field: "uuid" },
  { key: "tuic", field: "password" },
  { key: "hysteria2", field: "password" },
  { key: "trusttunnel", field: "password" },
]
const defaultAutoNamedFields: ReadonlyArray<AutoNamedFieldLocation> = [
  { key: "mixed", field: "username", aliases: ["name"] },
  { key: "socks", field: "username", aliases: ["name"] },
  { key: "http", field: "username", aliases: ["name"] },
  { key: "shadowsocks", field: "name" },
  { key: "shadowsocks16", field: "name" },
  { key: "shadowtls", field: "name", aliases: ["username"] },
  { key: "vmess", field: "name", aliases: ["username"] },
  { key: "vless", field: "name", aliases: ["username"] },
  { key: "anytls", field: "name", aliases: ["username"] },
  { key: "mieru", field: "username", aliases: ["name"] },
  { key: "trojan", field: "name", aliases: ["username"] },
  { key: "naive", field: "username", aliases: ["name"] },
  { key: "hysteria", field: "name", aliases: ["username"] },
  { key: "hysteria2", field: "name", aliases: ["username"] },
]
const mihomoAutoNamedFields: ReadonlyArray<AutoNamedFieldLocation> = [
  { key: "mixed", field: "username", aliases: ["name"] },
  { key: "socks", field: "username", aliases: ["name"] },
  { key: "http", field: "username", aliases: ["name"] },
  { key: "vmess", field: "username", aliases: ["name"] },
  { key: "vless", field: "username", aliases: ["name"] },
  { key: "anytls", field: "username", aliases: ["name"] },
  { key: "mieru", field: "username", aliases: ["name"] },
  { key: "snell", field: "name", aliases: ["username"] },
  { key: "trojan", field: "username", aliases: ["name"] },
  { key: "hysteria2", field: "username", aliases: ["name"] },
]
const defaultEditableUsernameFields = new Set<string>([
  "mixed",
  "socks",
  "http",
  "mieru",
  "naive",
])
const mihomoEditableUsernameFields = new Set<string>([
  "mixed",
  "socks",
  "http",
  "vmess",
  "vless",
  "anytls",
  "mieru",
  "trojan",
  "naive",
  "hysteria2",
  "trusttunnel",
])

const customCredentialProtocolsByNamespace: Record<ClientConfigNamespace, ReadonlySet<string>> = {
  default: new Set<string>([
    "anytls",
    "mieru",
    "trojan",
    "naive",
    "hysteria",
    "hysteria2",
    "tuic",
  ]),
  mihomo: new Set<string>([
    "anytls",
    "mieru",
    "snell",
    "trojan",
    "hysteria2",
    "tuic",
  ]),
}

function normalizeClientConfigNamespace(namespace?: string): ClientConfigNamespace {
  return namespace === "mihomo" ? "mihomo" : "default"
}

function normalizeStringValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : ""
}

function getUUIDStyleFields(namespace?: string): ReadonlyArray<ConfigFieldLocation> {
  return normalizeClientConfigNamespace(namespace) === "mihomo"
    ? mihomoUUIDStyleFields
    : defaultUUIDStyleFields
}

function getAutoNamedFields(namespace?: string): ReadonlyArray<AutoNamedFieldLocation> {
  return normalizeClientConfigNamespace(namespace) === "mihomo"
    ? mihomoAutoNamedFields
    : defaultAutoNamedFields
}

export function supportsEditableUsernameField(key: string, namespace?: string): boolean {
  const normalizedNamespace = normalizeClientConfigNamespace(namespace)
  return normalizedNamespace === "mihomo"
    ? mihomoEditableUsernameFields.has(key)
    : defaultEditableUsernameFields.has(key)
}

function getUUIDStyleFieldsForKey(key: string, namespace?: string): ConfigFieldLocation[] {
  return getUUIDStyleFields(namespace).filter((location) => location.key === key)
}

function collectReservedUUIDValues(
  configs: Config,
  namespace?: string,
  ignore: ConfigFieldLocation[] = [],
): Set<string> {
  const ignored = new Set(ignore.map((location) => `${location.key}.${location.field}`))
  const reserved = new Set<string>(sessionGeneratedUUIDs)

  getUUIDStyleFields(namespace).forEach(({ key, field }) => {
    if (ignored.has(`${key}.${field}`)) return
    const value = normalizeStringValue(configs[key]?.[field])
    if (value !== "") {
      reserved.add(value)
    }
  })

  return reserved
}

function randomUUIDExcluding(excludes?: Iterable<string>): string {
  const reserved = new Set<string>(sessionGeneratedUUIDs)
  if (excludes) {
    for (const value of excludes) {
      const normalizedValue = normalizeStringValue(value)
      if (normalizedValue !== "") {
        reserved.add(normalizedValue)
      }
    }
  }

  let value = RandomUtil.randomUUID()
  while (reserved.has(value)) {
    value = RandomUtil.randomUUID()
  }
  sessionGeneratedUUIDs.add(value)
  return value
}

function randomCredentialIDExcluding(excludes?: Iterable<string>): string {
  const reserved = new Set<string>(sessionGeneratedUUIDs)
  if (excludes) {
    for (const value of excludes) {
      const normalizedValue = normalizeStringValue(value)
      if (normalizedValue !== "") {
        reserved.add(normalizedValue)
      }
    }
  }

  let value = RandomUtil.randomCredentialID()
  while (reserved.has(value)) {
    value = RandomUtil.randomCredentialID()
  }
  sessionGeneratedUUIDs.add(value)
  return value
}

function randomShadowsocksPasswordExcluding(length: number, exclude?: string): string {
  let value = RandomUtil.randomShadowsocksPassword(length)
  while (exclude && value === exclude) {
    value = RandomUtil.randomShadowsocksPassword(length)
  }
  return value
}

function firstNonEmptyConfigValue(config: Config[string], fields: string[]): string {
  for (const field of fields) {
    const value = normalizeStringValue(config[field])
    if (value !== "") {
      return value
    }
  }
  return ""
}

function shouldFollowClientName(currentValue: string, oldUserName?: string): boolean {
  const previousName = normalizeStringValue(oldUserName)
  if (currentValue === "" || currentValue === "client") {
    return true
  }
  return previousName !== "" && currentValue === previousName
}

function syncAutoNamedConfigFields(
  configs: Config,
  newUserName: string,
  namespace?: string,
  oldUserName?: string,
): Config {
  const normalizedNewUserName = normalizeStringValue(newUserName)

  getAutoNamedFields(namespace).forEach(({ key, field, aliases = [] }) => {
    const config = configs[key]
    if (!config) return

    const value = firstNonEmptyConfigValue(config, [field, ...aliases])
    if (shouldFollowClientName(value, oldUserName)) {
      if (normalizedNewUserName !== "") {
        config[field] = newUserName
      } else {
        delete config[field]
      }
    } else if (value !== "") {
      config[field] = value
    }

    aliases.forEach((alias) => {
      if (alias !== field) {
        delete config[alias]
      }
    })
  })

  return configs
}

export interface Link {
  type: "local" | "external" | "sub"
  remark?: string
  uri: string
}

export interface Client {
  id?: number
	enable: boolean
	name: string
	config?: Config
	inbounds: number[]
  links?: Link[]
	volume: number
	expiry: number
  up: number
  down: number
  desc: string
  group: string
  serverIp: string  // Client outbound IP
  speedLimitMbps: number
  extra: number  // Traffic reset interval in days
  lastReset: number  // Last traffic reset timestamp
  trafficResetRequested?: boolean
}

const defaultClient: Client = {
  enable: true,
  name: "",
  config: {},
  inbounds: [],
  links: [],
  volume: 0,
  expiry: 0,
  up: 0,
  down: 0,
  desc: "",
  group: "",
  serverIp: "",  // Client outbound IP; empty means default
  speedLimitMbps: 0,
  extra: 0,  // Traffic reset interval in days; 0 means disabled
  lastReset: 0,  // Last traffic reset timestamp
  trafficResetRequested: false,
}

type Config = {
  [key: string]: {
    name?: string
    username?: string
    [key: string]: any
  }
}

function normalizeTrustTunnelClientConfig(configs: Config, userName: string, oldUserName?: string): Config {
  const config = configs.trusttunnel
  if (!config) return configs

  const rawUsername = firstNonEmptyConfigValue(config, ["username", "name", "uuid", "password"])
  const username = shouldFollowClientName(rawUsername, oldUserName)
    ? userName
    : rawUsername
  const legacyUuid = typeof config.uuid === 'string' ? config.uuid.trim() : ''
  const password = typeof config.password === 'string' && config.password.trim() !== ''
    ? config.password
    : legacyUuid

  configs.trusttunnel = {
    ...config,
  }
  if (username !== '') {
    configs.trusttunnel.username = username
  } else {
    delete configs.trusttunnel.username
  }
  if (password !== '') {
    configs.trusttunnel.password = password
  } else {
    delete configs.trusttunnel.password
  }
  delete configs.trusttunnel.name
  delete configs.trusttunnel.uuid

  return configs
}

function normalizeSudokuClientKey(value: unknown): string {
  if (typeof value !== 'string') return ''

  let normalized = value.trim()
  while (
    normalized.length >= 2 &&
    (
      (normalized.startsWith('"') && normalized.endsWith('"')) ||
      (normalized.startsWith("'") && normalized.endsWith("'"))
    )
  ) {
    normalized = normalized.slice(1, -1).trim()
  }

  return normalized
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .split(/\s+/)
    .filter((segment) => segment.length > 0)
    .join('')
}

function normalizeSudokuClientConfig(configs: Config): Config {
  const config = configs.sudoku
  if (!config) return configs

  const keyValue = normalizeSudokuClientKey(config.uuid)
  configs.sudoku = {
    ...config,
    uuid: keyValue,
  }

  return configs
}

export function updateConfigs(configs: Config, newUserName: string, oldUserName?: string, namespace?: string): Config {
  return normalizeSudokuClientConfig(
    normalizeTrustTunnelClientConfig(
      syncAutoNamedConfigFields(configs, newUserName, namespace, oldUserName),
      newUserName,
      oldUserName,
    ),
  )
}

function shouldUseCustomCredentialID(key: string, namespace?: string): boolean {
  return customCredentialProtocolsByNamespace[normalizeClientConfigNamespace(namespace)].has(key)
}

function assignUniqueUUIDField(configs: Config, key: string, field: string, namespace?: string) {
  if (!configs[key]) return
  const excludes = collectReservedUUIDValues(configs, namespace, [{ key, field }])
  configs[key][field] = shouldUseCustomCredentialID(key, namespace)
    ? randomCredentialIDExcluding(excludes)
    : randomUUIDExcluding(excludes)
}

function assignUniqueUUIDFieldsForKey(configs: Config, key: string, namespace?: string) {
  getUUIDStyleFieldsForKey(key, namespace).forEach(({ field }) => {
    assignUniqueUUIDField(configs, key, field, namespace)
  })
}

function applyUUIDStyleDefaults(configs: Config, namespace?: string): Config {
  getUUIDStyleFields(namespace).forEach(({ key, field }) => {
    assignUniqueUUIDField(configs, key, field, namespace)
  })
  return configs
}

const mihomoConfigKeys = [
  "mixed",
  "socks",
  "http",
  "snell",
  "shadowsocks",
  "shadowtls",
  "vmess",
  "vless",
  "anytls",
  "mieru",
  "sudoku",
  "trojan",
  "tuic",
  "hysteria2",
  "trusttunnel",
]

function sanitizeConfigsByNamespace(configs: Config, namespace?: string): Config {
  const normalizedNamespace = normalizeClientConfigNamespace(namespace)
  if (normalizedNamespace !== "mihomo") {
    delete configs.snell
    return configs
  }

  delete configs.hysteria
  delete configs.shadowsocks16
  delete configs.naive
  return configs
}

export function getConfigKeys(configs: Config, namespace?: string): string[] {
  const normalizedNamespace = normalizeClientConfigNamespace(namespace)
  const keys = Object.keys(configs)
  if (normalizedNamespace !== "mihomo") return keys

  const allowed = new Set(mihomoConfigKeys)
  const ordered = mihomoConfigKeys.filter((key) => allowed.has(key) && Object.hasOwn(configs, key))
  const extra = keys.filter((key) => allowed.has(key) && !ordered.includes(key))
  return [...ordered, ...extra]
}

export function shuffleConfigs(configs: Config, key?: string, namespace?: string) {
  const keys = key ? [key] : getConfigKeys(configs, namespace)
  keys.forEach(k => {
    switch (k) {
      case "mixed":
      case "socks":
      case "http":
        configs[k].username = RandomUtil.randomSeq(10)
        configs[k].password = RandomUtil.randomSeq(10)
        break
      case "mieru":
        configs[k].username = RandomUtil.randomSeq(10)
        assignUniqueUUIDFieldsForKey(configs, k, namespace)
        break
      case "snell":
        assignUniqueUUIDFieldsForKey(configs, k, namespace)
        break
      case "sudoku":
        assignUniqueUUIDFieldsForKey(configs, k, namespace)
        break
      case "anytls":
      case "trojan":
      case "naive":
      case "hysteria2":
        assignUniqueUUIDFieldsForKey(configs, k, namespace)
        break
      case "trusttunnel":
        assignUniqueUUIDFieldsForKey(configs, k, namespace)
        delete configs[k].uuid
        break
      case "shadowsocks":
        configs[k].password = randomShadowsocksPasswordExcluding(32, configs.shadowtls?.password)
        break
      case "shadowsocks16":
        configs[k].password = RandomUtil.randomShadowsocksPassword(16)
        break
      case "shadowtls":
        configs[k].password = randomShadowsocksPasswordExcluding(32, configs.shadowsocks?.password)
        break
      case "hysteria":
        assignUniqueUUIDFieldsForKey(configs, k, namespace)
        break
      case "tuic":
        assignUniqueUUIDFieldsForKey(configs, k, namespace)
        break
      case "vmess":
        assignUniqueUUIDFieldsForKey(configs, k, namespace)
        break
      case "vless":
        assignUniqueUUIDFieldsForKey(configs, k, namespace)
        break
    }
  })
}

export function randomConfigs(user: string, namespace?: string): Config {
  const normalizedNamespace = normalizeClientConfigNamespace(namespace)
  const mixedPassword = RandomUtil.randomSeq(10)
  const ssPassword16 = RandomUtil.randomShadowsocksPassword(16)
  const shadowsocksPassword = RandomUtil.randomShadowsocksPassword(32)
  const shadowtlsPassword = randomShadowsocksPasswordExcluding(32, shadowsocksPassword)
  const configs: Config = {
    mixed: {
      username: user,
      password: mixedPassword,
    },
    socks: {
      username: user,
      password: mixedPassword,
    },
    http: {
      username: user,
      password: mixedPassword,
    },
    shadowsocks: {
      name: user,
      password: shadowsocksPassword,
    },
    shadowsocks16: {
      name: user,
      password: ssPassword16,
    },
    shadowtls: {
      name: user,
      password: shadowtlsPassword,
    },
    vmess: {
      username: user,
      uuid: "",
      alterId: 0,
    },
    vless: {
      username: user,
      uuid: "",
      flow: "xtls-rprx-vision",
    },
    anytls: {
      username: user,
      password: mixedPassword,
    },
    mieru: {
      username: user,
      password: mixedPassword,
    },
    trojan: {
      username: user,
      password: mixedPassword,
    },
    naive: {
      username: user,
      password: mixedPassword,
    },
    hysteria: {
      name: user,
      auth_str: mixedPassword,
    },
    tuic: {
      uuid: "",
      password: mixedPassword,
    },
    hysteria2: {
      username: user,
      password: mixedPassword,
    },
  }

  if (normalizedNamespace === "mihomo") {
    configs.snell = {
      name: user,
      psk: "",
    }
    configs.sudoku = {
      uuid: "",
    }
    configs.trusttunnel = {
      username: user,
      password: "",
    }
  }

  return updateConfigs(
    normalizeSudokuClientConfig(sanitizeConfigsByNamespace(applyUUIDStyleDefaults(configs, namespace), namespace)),
    user,
    undefined,
    namespace,
  )
}

export function createClient<T extends Client>(json?: Partial<T>, namespace?: string): Client {
  defaultClient.name = RandomUtil.randomSeq(8)
  const defaultObject: Client = { ...defaultClient, ...(json || {}) }

  // Add missing config
  defaultObject.config = sanitizeConfigsByNamespace({
    ...randomConfigs(defaultObject.name, namespace),
    ...defaultObject.config,
  }, namespace)
  defaultObject.config = updateConfigs(defaultObject.config, defaultObject.name, undefined, namespace)
  
  return defaultObject
}
