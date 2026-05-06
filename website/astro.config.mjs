// @ts-check
import { defineConfig } from 'astro/config';

export default defineConfig({
  markdown: {
    shikiConfig: {
      theme: 'github-dark',
      wrap: true,
      transformers: [
        {
          preprocess(code, options) {
            options.decorations = (options.decorations || []).filter(
              d => !d.properties || d.properties.class !== 'shiki-ignore'
            );
            let offset = 0;
            const lines = code.split('\n');

            for (const line of lines) {
              const commentMatch = line.match(/#|\/\//);
              const codePart = commentMatch ? line.slice(0, commentMatch.index) : line;

              const regex = /\[.*?\]|<.*?>/g;
              let match;
              while ((match = regex.exec(codePart)) !== null) {
                options.decorations.push({
                  start: offset + match.index,
                  end: offset + match.index + match[0].length,
                  properties: { class: 'shiki-ignore' }
                });
              }
              offset += line.length + 1;
            }
          }
        }
      ]
    },
  },
});
