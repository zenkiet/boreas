<script lang="ts">
	import { ProjectsPage } from '@/pages/projects';

	let { data } = $props();
</script>

<ProjectsPage projects={data.projects} />
