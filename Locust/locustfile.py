from locust import HttpUser, task, between
import random
import string

CATEGORIAS = ["Electronica", "Ropa", "Hogar", "Belleza"]

def random_producto_id():
    # Ej: "TV-AB12"
    pref = random.choice(["TV", "LAP", "PHN", "SHO", "CRM", "HOM"])
    suf = "".join(random.choices(string.ascii_uppercase + string.digits, k=4))
    return f"{pref}-{suf}"

class VentasUser(HttpUser):
    wait_time = between(0.1, 0.8)  # tiempo entre requests (simula usuarios)

    @task
    def enviar_venta(self):
        payload = {
            "categoria": random.choice(CATEGORIAS),
            "productoId": random_producto_id(),
            "precio": round(random.uniform(1.0, 2500.0), 2),
            "cantidadVendida": random.randint(1, 10),
        }

        with self.client.post(
            "/ventas",
            json=payload,
            name="/ventas",
            catch_response=True,
            timeout=5,
        ) as resp:
            if resp.status_code != 200:
                resp.failure(f"HTTP {resp.status_code}: {resp.text}")
            else:
                data = resp.json()
                if data.get("estado") != "OK":
                    resp.failure(f"Respuesta inválida: {data}")
                else:
                    resp.success()
