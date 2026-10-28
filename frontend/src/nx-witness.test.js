import { test } from "node:test";
import assert from "node:assert/strict";
import { nxWitnessButton, nxValue, nxDetails } from "./nx-witness.js";
import { importSummary } from "./site-transfer.js";

test("NX Witness controls exist only for a stored NX snapshot", () => {
  for (const view of [null, {}, {hasNXWitness:false}]) assert.equal(nxWitnessButton(view), "");
  assert.match(nxWitnessButton({hasNXWitness:true}), /id="site-nx-witness"/);
});
test("NX information preserves unknown, false, zero and large exact integers", () => {
  assert.equal(nxValue(null), "Unknown");
  assert.equal(nxValue(false), "No");
  assert.equal(nxValue("0"), "0");
  assert.equal(nxValue("9223372036854775807"), "9223372036854775807");
  assert.equal(nxValue([]), "None recorded");
});
test("NX details escape imported text and put licence secrets behind disclosure controls", () => {
  const html=nxDetails({name:'<img src=x onerror="alert(1)">',key:"SYNTHETIC-key",license_block:"<script>fixture</script>",properties:{cpuModelName:"<b>fixture</b>"}});
  assert.doesNotMatch(html, /<img|<script|<b>/);
  assert.match(html, /<summary>Show licence key<\/summary>/);
  assert.match(html, /&lt;script&gt;/);
});
test("import review shows only NX counts when present", () => {
  const plain=importSummary({warnings:[]});
  assert.doesNotMatch(plain, /NX Witness/);
  const withNX=importSummary({nxWitness:{servers:2,users:3,storage:4,licenses:0},warnings:[]});
  assert.match(withNX, /NX Witness servers<\/td><td>2/);
  assert.match(withNX, /NX Witness licences<\/td><td>0/);
});
