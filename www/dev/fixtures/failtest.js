// Gate self-test: renders fine, its behaviour test fails. Must FAIL.
export { default } from './cube.js';
export async function test({ assert }) { assert(false, 'failing on purpose'); }
