// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import mermaid from 'astro-mermaid';
import { ebnfLanguage } from './src/expressive-code/ebnf-language.mjs';
import { maceLanguage } from './src/expressive-code/mace-language.mjs';

// https://astro.build/config
export default defineConfig({
	site: 'https://mace-docs.onrender.com',
	integrations: [
		mermaid({ enableLog: false }),
		starlight({
			title: 'Mace Docs',
			logo: {
				src: './src/assets/mace-brand.png',
				alt: 'Mace',
			},
			customCss: ['./src/styles/brand.css'],
			social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/louiss0/mace' }],
			expressiveCode: {
				themes: ['material-theme-darker', 'material-theme-lighter'],
				shiki: {
					langs: [maceLanguage, ebnfLanguage],
					injectLangsIntoNestedCodeBlocks: true,
				},
			},
			sidebar: [
				{
					label: 'Installation',
					items: [
						{ label: 'Install Mace', link: '/installation/' },
						{ label: 'Language Bindings', link: '/installation/bindings/' },
					],
				},
				{
					label: 'Tour',
					items: [
						{ label: 'Motivation', link: '/tour/motivation/' },
						{ label: 'Block Overview', link: '/tour/block-overview/' },
						{ label: 'Type System Overview', link: '/tour/type-system-overview/' },
						{ label: 'Script Block', link: '/tour/script-block/' },
						{ label: 'Output Block', link: '/tour/output-block/' },
						{ label: 'Doc Syntax', link: '/tour/doc-syntax/' },
					],
				},
				{
					label: 'Reference',
					items: [
						{ autogenerate: { directory: 'reference' } }
					],
				},
				{
					label: 'Tutorials',
					items: [{ autogenerate: { directory: 'tutorials' } }],
				},
				{
					label: 'How-to Guides',
					items: [{ autogenerate: { directory: 'how-to' } }],
				},
			],
		}),
	],
});
