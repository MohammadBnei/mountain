// Gate self-test: build() throws. Must FAIL.
export default async function build() {
  throw new Error('fixture: broken on purpose');
}
