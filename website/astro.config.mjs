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
            options.decorations = options.decorations || [];
            const regex = /\[.*?\]|<.*?>/g;
            let match;
            while ((match = regex.exec(code)) !== null) {
              options.decorations.push({
                start: match.index,
                end: match.index + match[0].length,
                properties: { class: 'shiki-ignore' }
              });
            }
          }
        }
      ]
    },
  },
});
