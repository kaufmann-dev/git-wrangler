// src/content.config.ts – Astro v6 Content Collections schema (new format)
import { defineCollection, z } from 'astro:content';
import { glob } from 'astro/loaders';

const docs = defineCollection({
  loader: glob({ pattern: '**/*.{md,mdx}', base: './src/content/docs' }),
  schema: z.object({
    title: z.string(),
    description: z.string().optional(),
    category: z.enum([
      'General',
      'Utility',
      'Local Operations',
      'Remote Operations',
      'AI Commands',
      'History Rewriting',
    ]).default('General'),
    order: z.number().optional(),
    usage: z.string().optional(),
    badge: z.string().optional(),
  }),
});

export const collections = { docs };
