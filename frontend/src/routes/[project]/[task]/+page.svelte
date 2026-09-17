<script lang="ts">
	import { TaskStatusPage } from '@/pages/task-status';

	let { data } = $props();
</script>

<TaskStatusPage slug={data.slug} task={data.task} status={data.status} />
