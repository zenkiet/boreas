<script lang="ts">
	import { ProjectDetailPage } from '@/pages/project-detail';

	let { data } = $props();
</script>

<ProjectDetailPage slug={data.slug} projects={data.projects} />
