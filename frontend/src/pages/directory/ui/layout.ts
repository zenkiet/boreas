export const HEAD =
	'mb-5 flex flex-wrap items-start justify-between gap-x-8 gap-y-4 @max-[720px]/main:mb-3.5 @max-[720px]/main:gap-3';

export const HEAD_TEXT = 'min-w-0 flex-[1_1_360px]';

export const HEAD_SIDE =
	'flex min-w-65 flex-[0_1_340px] flex-col gap-2.5 pt-0.5 @max-[720px]/main:min-w-0 @max-[720px]/main:flex-[1_1_100%] @max-[720px]/main:pt-0';

export const TITLE = 'min-w-0 text-title1 wrap-anywhere @max-[720px]/main:text-large';

export const CARD = 'overflow-hidden rounded-box bg-surface shadow-1';

const PHONE_COLS = '@max-[720px]/main:[--cols:16px_auto_auto_minmax(0,1fr)_auto]';

export const ROWS = {
	project: `[--cols:16px_112px_minmax(0,1fr)_164px_92px_176px_40px_32px] @max-[1000px]/main:[--cols:16px_104px_minmax(0,1fr)_148px_84px_156px] ${PHONE_COLS} @max-[720px]/main:[--areas:'g_id_id_id_time'_'._desc_desc_desc_desc'_'._proj_people_pipe_pipe']`,
	plain: `[--cols:16px_112px_minmax(0,1fr)_92px_176px_40px_32px] @max-[1000px]/main:[--cols:16px_104px_minmax(0,1fr)_84px_156px] ${PHONE_COLS} @max-[720px]/main:[--areas:'g_id_id_id_time'_'._desc_desc_desc_desc'_'._people_pipe_pipe_pipe']`
};

export const COLHEAD =
	'mb-1 grid h-7 grid-cols-(--cols) items-center gap-4 px-4 text-caption text-label-3 @max-[720px]/main:hidden';

export const ROW =
	'relative grid min-h-13 grid-cols-(--cols) items-center gap-4 px-4 py-2 [grid-template-areas:var(--areas)] not-first:before:absolute not-first:before:top-0 not-first:before:right-0 not-first:before:left-12 not-first:before:h-px not-first:before:bg-sep @max-[720px]/main:min-h-0 @max-[720px]/main:gap-x-2.5 @max-[720px]/main:gap-y-1.5 @max-[720px]/main:py-3 @max-[720px]/main:not-first:before:left-[42px]';

export const AREA = {
	glyph: '@max-[720px]/main:[grid-area:g]',
	id: '@max-[720px]/main:[grid-area:id]',
	desc: '@max-[720px]/main:[grid-area:desc]',
	proj: '@max-[720px]/main:[grid-area:proj]',
	people: '@max-[720px]/main:[grid-area:people]',
	pipe: '@max-[720px]/main:[grid-area:pipe] @max-[720px]/main:justify-end',
	time: 'text-right @max-[1000px]/main:hidden @max-[720px]/main:block @max-[720px]/main:[grid-area:time]',
	open: '@max-[1000px]/main:hidden'
};
