document.getElementById('btn').addEventListener('click', async () => {
    const output = document.getElementById('output');
    output.textContent = 'Fetching /api/status...';
    try {
        const res = await fetch('/api/status');
        const data = await res.json();
        output.textContent = JSON.stringify(data, null, 2);
    } catch (err) {
        output.textContent = 'Error calling API: ' + err.message;
    }
});
