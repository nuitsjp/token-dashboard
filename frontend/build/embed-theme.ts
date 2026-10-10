import { parseFragment } from 'parse5';
import { isTokenCloseParen, isTokenFunction, isTokenString, isTokenURL, isTokenWhiteSpaceOrComment, tokenize } from '@csstools/css-tokenizer';

type Replacement = { start: number; end: number; value: string };
export type EmbedImage = (reference: string, sourcePath: string) => string;

function replace(source: string, replacements: Replacement[]) {
  for (const item of replacements.sort((a, b) => b.start - a.start)) {
    source = source.slice(0, item.start) + item.value + source.slice(item.end);
  }
  return source;
}

export function embedTemplateImages(source: string, sourcePath: string, embedImage: EmbedImage) {
  const fragment = parseFragment(source, { sourceCodeLocationInfo: true });
  const replacements: Replacement[] = [];
  function visit(node: typeof fragment | (typeof fragment.childNodes)[number]) {
    if ('tagName' in node && node.tagName === 'img') {
      const src = node.attrs.find(attribute => attribute.name === 'src');
      const location = node.sourceCodeLocation?.attrs?.src;
      if (src && location) {
        const embedded = embedImage(src.value, sourcePath);
        if (embedded !== src.value) replacements.push({ start: location.startOffset, end: location.endOffset, value: `src="${embedded}"` });
      }
    }
    if ('childNodes' in node) node.childNodes.forEach(visit);
    if ('content' in node) visit(node.content);
  }
  visit(fragment);
  return replace(source, replacements);
}

export function embedStylesheetImages(source: string, sourcePath: string, embedImage: EmbedImage) {
  const tokens = tokenize({ css: source }, { onParseError(error) { throw new Error(`Invalid theme CSS in ${sourcePath}: ${error.message}`); } });
  const replacements: Replacement[] = [];
  function image(reference: string) {
    if (reference.includes('{{')) throw new Error(`Dynamic CSS image URLs are unsupported: ${sourcePath}`);
    return embedImage(reference, sourcePath);
  }
  for (let index = 0; index < tokens.length; index++) {
    const token = tokens[index];
    if (isTokenURL(token)) {
      const embedded = image(token[4].value);
      if (embedded !== token[4].value) replacements.push({ start: token[2], end: token[3] + 1, value: `url("${embedded}")` });
    } else if (isTokenFunction(token) && token[4].value.toLowerCase() === 'url') {
      let valueIndex = index + 1;
      while (isTokenWhiteSpaceOrComment(tokens[valueIndex])) valueIndex++;
      const value = tokens[valueIndex];
      let closeIndex = valueIndex + 1;
      while (isTokenWhiteSpaceOrComment(tokens[closeIndex])) closeIndex++;
      if (!isTokenString(value) || !isTokenCloseParen(tokens[closeIndex])) throw new Error(`Invalid theme CSS url() in ${sourcePath}`);
      const embedded = image(value[4].value);
      if (embedded !== value[4].value) replacements.push({ start: value[2], end: value[3] + 1, value: `"${embedded}"` });
    }
  }
  return replace(source, replacements);
}
