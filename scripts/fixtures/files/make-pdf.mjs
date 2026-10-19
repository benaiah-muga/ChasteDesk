// SPDX-License-Identifier: AGPL-3.0-only
// Generates a minimal single-page PDF fixture with correct xref offsets.
import { writeFileSync } from "node:fs";

const text = "ChasteDesk password reset guide: use the security settings page to reset your credential and billing plan notes.";
const content = `BT /F1 12 Tf 72 720 Td (${text.replace(/([()\\])/g, "\\$1")}) Tj ET`;
const objects = [
  "<< /Type /Catalog /Pages 2 0 R >>",
  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
  `<< /Length ${content.length} >>\nstream\n${content}\nendstream`,
  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
];

let body = "%PDF-1.4\n";
const offsets = [];
objects.forEach((obj, i) => {
  offsets.push(body.length);
  body += `${i + 1} 0 obj\n${obj}\nendobj\n`;
});
const xrefStart = body.length;
let xref = `xref\n0 ${objects.length + 1}\n0000000000 65535 f \n`;
for (const off of offsets) {
  xref += `${String(off).padStart(10, "0")} 00000 n \n`;
}
body += xref + `trailer\n<< /Size ${objects.length + 1} /Root 1 0 R >>\nstartxref\n${xrefStart}\n%%EOF\n`;

writeFileSync(new URL("./notes.pdf", import.meta.url), body, "latin1");
console.log("notes.pdf written");
