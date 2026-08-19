const fs = require('fs');
const path = require('path');

const filesToPatch = [
  path.join(__dirname, 'node_modules', 'next', 'dist', 'build', 'utils.js'),
  path.join(__dirname, 'node_modules', 'next', 'dist', 'esm', 'build', 'utils.js')
];

for (const filePath of filesToPatch) {
  if (fs.existsSync(filePath)) {
    let content = fs.readFileSync(filePath, 'utf8');
    if (content.includes("const hostname = process.env.HOSTNAME || '0.0.0.0'")) {
      content = content.replace(
        "const hostname = process.env.HOSTNAME || '0.0.0.0'",
        "const hostname = '0.0.0.0'"
      );
      fs.writeFileSync(filePath, content, 'utf8');
      console.log(`[patch-next] Patched ${filePath} to bind hostname explicitly to '0.0.0.0'`);
    } else if (content.includes("const hostname = '0.0.0.0'")) {
      console.log(`[patch-next] Already patched in ${filePath}`);
    } else {
      console.log(`[patch-next] Target pattern not found in ${filePath}`);
    }
  }
}
