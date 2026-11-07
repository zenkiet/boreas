import type { PublicTask } from '#shared/api';
import { agoLong } from '#shared/lib';

export type PipeKind = 'run' | 'ok' | 'bad' | 'cancel' | 'none' | 'wait';

export interface Pipe {
	readonly kind: PipeKind;
	readonly text: string;
	readonly progress?: number;
}

export interface PipeLine extends Pipe {
	readonly when: string;
}

const percent = (progress?: number) => (progress ? ` · ${progress}%` : '');

const suffix = (joiner: string, stage?: string) => (stage ? `${joiner}${stage}` : '');

export const pipelineOf = ({ build, lastDeploy }: PublicTask): Pipe => {
	if (build?.state === 'running') {
		const text = (build.stage || 'Building') + percent(build.progress);
		return { kind: 'run', text, progress: build.progress };
	}
	if (
		lastDeploy?.status === 'failure' &&
		(!build || Date.parse(lastDeploy.at) >= Date.parse(build.at))
	) {
		return { kind: 'bad', text: 'Deploy failed' };
	}
	if (build?.state === 'failure')
		return { kind: 'bad', text: `Build failed${suffix(' · ', build.stage)}` };
	if (build?.state === 'canceled')
		return { kind: 'cancel', text: `Canceled${suffix(' at ', build.stage)}` };
	if (lastDeploy) return { kind: 'ok', text: 'Deployed' };
	if (build) return { kind: 'ok', text: 'Passed' };
	return { kind: 'none', text: 'No pipeline' };
};

export const buildLine = ({ build }: PublicTask, now: number): PipeLine => {
	if (!build) return { kind: 'none', text: 'No CI report yet', when: '' };
	const when = agoLong(build.at, now);
	switch (build.state) {
		case 'running': {
			const text = (build.stage || 'Running') + percent(build.progress);
			return { kind: 'run', text, progress: build.progress, when: `started ${when}` };
		}
		case 'success':
			return { kind: 'ok', text: 'Passed', when };
		case 'failure':
			return { kind: 'bad', text: `Failed${suffix(' at ', build.stage)}`, when };
		default:
			return { kind: 'cancel', text: `Canceled${suffix(' at ', build.stage)}`, when };
	}
};

export const deployLine = ({ build, lastDeploy }: PublicTask, now: number): PipeLine => {
	if (build?.state === 'running') {
		const when = lastDeploy ? `last ${agoLong(lastDeploy.at, now)}` : '';
		return { kind: 'wait', text: 'Runs after the build', when };
	}
	if (!lastDeploy) return { kind: 'none', text: 'Not deployed yet', when: '' };
	const when = agoLong(lastDeploy.at, now);
	return lastDeploy.status === 'success'
		? { kind: 'ok', text: 'Succeeded', when }
		: { kind: 'bad', text: 'Failed', when };
};
