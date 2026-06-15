const seq = '0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('')
const nonZeroDigits = '123456789'.split('')
const letters = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('')
const nonZeroAlphaNumeric = [...nonZeroDigits, ...letters]
const credentialSegments = [8, 4, 4, 4, 12]

const RandomUtil = {
  randomIntRange(min: number, max: number): number {
    if (!Number.isSafeInteger(min)){
      return this.randomIntRange(Number.MIN_SAFE_INTEGER, max)
    }
    if (!Number.isSafeInteger(max)){
      return this.randomIntRange(min, Number.MAX_SAFE_INTEGER)
    }
    if (max < min) {
      return this.randomIntRange(max, min)
    }
    const array = new Uint32Array(2);
    window.crypto.getRandomValues(array);
    const highbits = array[0]
    const lowbits = array[1] >>> 11
    const random = (highbits * 2 ** 21 + lowbits) / (Number.MAX_SAFE_INTEGER + 1)
    return Math.floor(random * (max - min + 1) + min)
  },
  randomInt(n: number) {
    return this.randomIntRange(0, n)
  },
  randomSeq(count: number): string {
    if (count <= 0) {
      return ''
    }
    let str = ''
    for (let i = 0; i < count; ++i) {
        str += seq[this.randomInt(seq.length - 1)]
    }
    return str
  },
  randomLowerAndNum(count: number): string {
    if (count <= 0) {
      return ''
    }
    let str = ''
    for (let i = 0; i < count; ++i) {
        str += seq[this.randomInt(35)]
    }
    return str
  },
  randomUUID(): string {
    const rng = new Uint8Array(16);
    window.crypto.getRandomValues(rng);
    rng[6] = (rng[6] & 0x0f) | 0x40;
    rng[8] = (rng[8] & 0x3f) | 0x80;
    return (
      byteToHex[rng[0]] + byteToHex[rng[1]] + byteToHex[rng[2]] + byteToHex[rng[3]] + '-' +
      byteToHex[rng[4]] + byteToHex[rng[5]] + '-' +
      byteToHex[rng[6]] + byteToHex[rng[7]] + '-' +
      byteToHex[rng[8]] + byteToHex[rng[9]] + '-' +
      byteToHex[rng[10]] + byteToHex[rng[11]] + byteToHex[rng[12]] +
      byteToHex[rng[13]] + byteToHex[rng[14]] + byteToHex[rng[15]]
    );
  },
  randomCredentialID(): string {
    const chars: string[] = []

    for (let i = 0; i < 11; ++i) {
      chars.push(nonZeroDigits[this.randomInt(nonZeroDigits.length - 1)])
    }
    for (let i = 0; i < 15; ++i) {
      chars.push(letters[this.randomInt(letters.length - 1)])
    }
    for (let i = chars.length; i < 32; ++i) {
      chars.push(nonZeroAlphaNumeric[this.randomInt(nonZeroAlphaNumeric.length - 1)])
    }

    for (let i = chars.length - 1; i > 0; --i) {
      const j = this.randomInt(i)
      ;[chars[i], chars[j]] = [chars[j], chars[i]]
    }

    let offset = 0
    return credentialSegments.map((segmentLength) => {
      const segment = chars.slice(offset, offset + segmentLength).join('')
      offset += segmentLength
      return segment
    }).join('-')
  },
  randomShadowsocksPassword(n: number): string {
    const array = new Uint8Array(n)
    window.crypto.getRandomValues(array)
    return btoa(String.fromCharCode(...array))
  },
  randomShortId(): string[] {
    let shortIds = new Array(24).fill('')
    for (var ii = 1; ii < 24; ii++) {
      for (var jj = 0; jj <= this.randomInt(7); jj++){
          let randomNum = this.randomInt(255)
          shortIds[ii] += ('0' + randomNum.toString(16)).slice(-2)
      }
  }
  return shortIds
  }
}

const byteToHex = Array.from(
  { length: 256 },
  (_, i) => (i + 0x100)
    .toString(16)
    .slice(1)
)

export default RandomUtil
