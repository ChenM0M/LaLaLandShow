fetch('http://localhost:8080/api/student/login', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({student_id: '1234567'})
})
.then(r => r.json())
.then(res => {
  const token = res.token;
  return fetch('http://localhost:8080/api/student/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
    body: JSON.stringify({ class_code: '903', seat_no_in_class: 12 })
  });
})
.then(async r => {
  console.log('status:', r.status);
  console.log('response:', await r.text());
})
.catch(err => console.error(err));
